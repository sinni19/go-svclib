// Package svc hosts services and owns their lifecycle.
//
// An App is configured once with New, runs once with Launch, and stops with
// Shutdown, SIGINT, or SIGTERM. Main is the usual entry point from package main.
package svc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"github.com/sinni19/go-svclib/config"
	svclog "github.com/sinni19/go-svclib/log"
)

var (
	// ErrInvalidConfig wraps every validation error that New returns.
	ErrInvalidConfig = errors.New("svc: invalid config")

	// ErrAlreadyLaunched is returned by Launch on an App that was launched or shut down.
	ErrAlreadyLaunched = errors.New("svc: app already launched")

	errShutdownRequested = errors.New("svc: shutdown requested")
	errSignal            = errors.New("svc: signal received")
	errPanic             = errors.New("svc: panic")
	errAbandoned         = errors.New("abandoned")
)

var (
	nameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	// semverRE is the semver.org 2.0.0 grammar with an optional "v" prefix.
	semverRE = regexp.MustCompile(`^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
		`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?` +
		`(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)
)

// App hosts services and runs their lifecycle.
type App struct {
	opts   options
	logger *slog.Logger
	level  *slog.LevelVar

	// stopCtx is canceled when a stop is requested. Its cause tells why.
	stopCtx     context.Context
	requestStop context.CancelCauseFunc

	// done is closed when Launch returns.
	done chan struct{}

	mu       sync.Mutex
	status   Status
	launched bool
	// shutdownCtx is the context of the first Shutdown call. It can shorten
	// the drain and the stop.
	shutdownCtx context.Context
	stopErr     error
}

// New validates the configuration and returns an App in StatusCreated.
// It does no I/O. Values come from opts, then SVC_* env variables, then defaults.
// All validation errors are returned together, each wrapping ErrInvalidConfig.
func New(opts ...Option) (*App, error) {
	e, err := config.Load(nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}

	o := options{
		name:            e.Name,
		version:         e.Version,
		commit:          e.Commit,
		logLevel:        e.LogLevel,
		logFormat:       e.LogFormat,
		logWriter:       os.Stdout,
		startupTimeout:  e.StartupTimeout,
		drainDelay:      e.DrainDelay,
		shutdownTimeout: e.ShutdownTimeout,
		hookGracePeriod: e.HookGracePeriod,
		handleSignals:   e.HandleSignals,
	}

	for _, opt := range opts {
		opt(&o)
	}

	level, format, err := validate(&o)
	if err != nil {
		return nil, err
	}

	lv := new(slog.LevelVar)
	lv.Set(level)

	a := &App{
		opts:   o,
		level:  lv,
		logger: svclog.New(o.logWriter, format, lv).With("app", o.name, "version", o.version),
		done:   make(chan struct{}),
		status: StatusCreated,
	}
	a.stopCtx, a.requestStop = context.WithCancelCause(context.Background())

	return a, nil
}

func validate(o *options) (slog.Level, svclog.Format, error) {
	var errs []error

	invalid := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("%w: "+format, append([]any{ErrInvalidConfig}, args...)...))
	}

	if !nameRE.MatchString(o.name) {
		invalid("name %q must match %s", o.name, nameRE)
	}

	if !semverRE.MatchString(o.version) {
		invalid("version %q is not a semantic version", o.version)
	}

	if o.commit == "" {
		invalid("commit is required")
	}

	level, err := svclog.ParseLevel(o.logLevel)
	if err != nil {
		invalid("%v", err)
	}

	format, err := svclog.ParseFormat(o.logFormat)
	if err != nil {
		invalid("%v", err)
	}

	if o.logWriter == nil {
		invalid("log writer is nil")
	}

	if o.startupTimeout <= 0 {
		invalid("startup timeout %v must be positive", o.startupTimeout)
	}

	if o.drainDelay < 0 {
		invalid("drain delay %v must not be negative", o.drainDelay)
	}

	if o.shutdownTimeout <= 0 {
		invalid("shutdown timeout %v must be positive", o.shutdownTimeout)
	}

	if o.hookGracePeriod <= 0 {
		invalid("hook grace period %v must be positive", o.hookGracePeriod)
	}

	seen := make(map[string]bool, len(o.services))

	for i, s := range o.services {
		switch {
		case s == nil:
			invalid("service %d is nil", i)
		case s.Name() == "":
			invalid("service %d has an empty name", i)
		case seen[s.Name()]:
			invalid("service name %q is used more than once", s.Name())
		default:
			seen[s.Name()] = true
		}
	}

	return level, format, errors.Join(errs...)
}

// Logger returns the App logger. It has the attributes app and version.
func (a *App) Logger() *slog.Logger { return a.logger }

// Status returns the current lifecycle state.
func (a *App) Status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.status
}

func (a *App) setStatus(s Status) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.status = s
}

// Launch runs the App and blocks until it is stopped.
//
// It initializes and starts all services, waits for a stop request, then
// drains and stops them. A stop request is Shutdown, SIGINT, or SIGTERM (see
// HandleSignals). Launch returns nil after a requested stop, also one that
// interrupts startup. It returns an error when a hook fails, panics, or
// exceeds its timeout. Launch runs at most once per App.
func (a *App) Launch() error {
	if err := a.markLaunched(); err != nil {
		return err
	}
	defer close(a.done)

	if a.opts.handleSignals {
		defer a.watchSignals()()
	}

	a.logger.Info("app launching", "commit", a.opts.commit, "go", runtime.Version())

	initialized, startErr := a.startup()

	if startErr == nil {
		a.setStatus(StatusRunning)
		a.logger.Info("app running")

		<-a.stopCtx.Done()
		a.drain()
	} else if a.stopCtx.Err() != nil {
		// The stop request canceled the startup context; the hook error is a consequence.
		a.logger.Info("startup interrupted by stop request", "cause", context.Cause(a.stopCtx))

		startErr = nil
	}

	stopErr := a.stopServices(initialized)

	a.mu.Lock()
	a.status = StatusStopped
	a.stopErr = stopErr
	a.mu.Unlock()

	err := errors.Join(startErr, stopErr)
	if err != nil {
		a.logger.Error("app stopped with error", "error", err)
	} else {
		a.logger.Info("app stopped")
	}

	return err
}

func (a *App) markLaunched() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.launched {
		return ErrAlreadyLaunched
	}

	a.launched = true

	return nil
}

// watchSignals requests a stop on SIGINT or SIGTERM. The returned func ends
// the watch and must be called before Launch returns.
func (a *App) watchSignals() (stop func()) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	quit := make(chan struct{})
	exited := make(chan struct{})

	go func() {
		defer close(exited)

		select {
		case s := <-sig:
			a.logger.Info("signal received", "signal", s.String())
			a.requestStop(fmt.Errorf("%w: %s", errSignal, s))
		case <-quit:
		}
	}()

	return func() {
		signal.Stop(sig)
		close(quit)
		<-exited
	}
}

// startup runs OnInit, then OnStart, for every service in order. It returns
// how many services completed OnInit; those services get OnStop.
func (a *App) startup() (initialized int, err error) {
	ctx, cancel := context.WithTimeout(a.stopCtx, a.opts.startupTimeout)
	defer cancel()

	a.setStatus(StatusInitializing)

	for _, s := range a.opts.services {
		deps := &Deps{Logger: a.logger.With("service", s.Name())}
		if err := a.call(ctx, s, "OnInit", func(ctx context.Context) error { return s.OnInit(ctx, deps) }); err != nil {
			return initialized, err
		}

		initialized++
	}

	a.setStatus(StatusStarting)

	for _, s := range a.opts.services {
		if err := a.call(ctx, s, "OnStart", s.OnStart); err != nil {
			return initialized, err
		}
	}

	return initialized, nil
}

// drain keeps the App in StatusDraining for DrainDelay, so that health checks
// report not ready before services stop. A Shutdown context can end it early.
func (a *App) drain() {
	a.setStatus(StatusDraining)
	a.logger.Info("app draining", "cause", context.Cause(a.stopCtx), "delay", a.opts.drainDelay)

	t := time.NewTimer(a.opts.drainDelay)
	defer t.Stop()

	select {
	case <-t.C:
	case <-a.callerCtx().Done():
	}
}

// stopServices runs OnStop for the first n services in reverse order.
func (a *App) stopServices(n int) error {
	a.setStatus(StatusStopping)

	ctx, cancel := context.WithTimeout(a.callerCtx(), a.opts.shutdownTimeout)
	defer cancel()

	var errs []error

	for i := n - 1; i >= 0; i-- {
		s := a.opts.services[i]
		if err := a.call(ctx, s, "OnStop", s.OnStop); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// callerCtx returns the context of the first Shutdown call, or Background
// when the stop came from a signal.
func (a *App) callerCtx() context.Context {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.shutdownCtx != nil {
		return a.shutdownCtx
	}

	return context.Background()
}

// call runs one hook and waits at most until ctx ends.
//
// A hook that is still running when ctx ends is abandoned: call returns a
// timeout error and the hook's goroutine keeps running until it returns or the
// process exits. A hook that returns nil after ctx ended also fails, so a
// timeout is never silent.
//
// If ctx already ended before the call, the deadline belongs to an earlier
// hook. The hook still runs, so that it can release its resources, with a
// fresh HookGracePeriod budget.
//
// A panic becomes an error; its stack goes to the log only.
func (a *App) call(ctx context.Context, s Service, hook string, fn func(context.Context) error) error {
	if ctx.Err() != nil {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeout(context.WithoutCancel(ctx), a.opts.hookGracePeriod)
		defer cancel()
	}

	result := make(chan error, 1) // buffered: an abandoned hook must not block on send

	go func() {
		defer func() {
			if r := recover(); r != nil {
				a.logger.ErrorContext(ctx, "hook panicked",
					"service", s.Name(), "hook", hook, "panic", r, "stack", string(debug.Stack()))
				result <- fmt.Errorf("%w: %v", errPanic, r)
			}
		}()

		result <- fn(ctx)
	}()

	var err error

	select {
	case err = <-result:
	case <-ctx.Done():
		// select picks at random when both are ready; a finished hook is not abandoned.
		select {
		case err = <-result:
		default:
			a.logger.ErrorContext(ctx, "hook abandoned: still running at its deadline",
				"service", s.Name(), "hook", hook, "cause", context.Cause(ctx))

			return fmt.Errorf("service %q: %s: %w: %w", s.Name(), hook, errAbandoned, ctx.Err())
		}
	}

	if err == nil {
		err = ctx.Err()
	}

	if err != nil {
		return fmt.Errorf("service %q: %s: %w", s.Name(), hook, err)
	}

	return nil
}

// Shutdown requests a stop and waits until Launch returns or ctx ends.
//
// It returns the OnStop errors, or ctx.Err() if ctx ends first. ctx also
// limits the drain and OnStop, so it can shorten DrainDelay and
// ShutdownTimeout. Only the first call's ctx is used for that. Shutdown on an
// App that was never launched moves it to StatusStopped and returns nil.
func (a *App) Shutdown(ctx context.Context) error {
	a.mu.Lock()

	if !a.launched {
		a.launched = true
		a.status = StatusStopped
		a.mu.Unlock()
		a.requestStop(errShutdownRequested)

		return nil
	}

	if a.shutdownCtx == nil {
		a.shutdownCtx = ctx
	}
	a.mu.Unlock()

	a.requestStop(errShutdownRequested)

	select {
	case <-a.done:
		a.mu.Lock()
		defer a.mu.Unlock()

		return a.stopErr
	case <-ctx.Done():
		return fmt.Errorf("svc: shutdown: %w", ctx.Err())
	}
}

// Main launches a and exits the process: status 0 after a clean stop, 1 after
// an error. Call it last in func main.
func Main(a *App) {
	os.Exit(run(a, os.Stderr))
}

func run(a *App, stderr io.Writer) int {
	if err := a.Launch(); err != nil {
		// The App logger already reported err; stderr helps when logs go elsewhere.
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", a.opts.name, err)

		return 1
	}

	return 0
}
