package svc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// recorder collects hook calls from all services of one App, in call order.
type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) add(e string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, e)
}

func (r *recorder) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.events)
}

// fakeService records its hooks and returns the configured errors.
type fakeService struct {
	BaseService

	name string
	rec  *recorder

	initErr, startErr, stopErr error
	panicIn                    string
	// block makes the named hook wait until its context ends, then return nil.
	block string
	deps  *Deps
}

func (f *fakeService) Name() string { return f.name }

func (f *fakeService) hook(ctx context.Context, name string, err error) error {
	f.rec.add(f.name + "." + name)

	if f.panicIn == name {
		panic("boom")
	}

	if f.block == name {
		<-ctx.Done()

		return nil
	}

	return err
}

func (f *fakeService) OnInit(ctx context.Context, d *Deps) error {
	f.deps = d

	return f.hook(ctx, "OnInit", f.initErr)
}

func (f *fakeService) OnStart(ctx context.Context) error { return f.hook(ctx, "OnStart", f.startErr) }
func (f *fakeService) OnStop(ctx context.Context) error  { return f.hook(ctx, "OnStop", f.stopErr) }

func baseOpts(extra ...Option) []Option {
	return append([]Option{
		Name("test-app"), Version("1.0.0"), Commit("abc123"),
		LogWriter(io.Discard), HandleSignals(false), DrainDelay(0),
	}, extra...)
}

func newApp(t *testing.T, extra ...Option) *App {
	t.Helper()

	a, err := New(baseOpts(extra...)...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return a
}

// launch starts a in a goroutine, waits for want, and returns the Launch result channel.
func launch(t *testing.T, a *App, want Status) <-chan error {
	t.Helper()

	res := make(chan error, 1)

	go func() { res <- a.Launch() }()

	waitStatus(t, a, want)

	return res
}

func waitStatus(t *testing.T, a *App, want Status) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for a.Status() != want {
		if time.Now().After(deadline) {
			t.Fatalf("status = %v, want %v", a.Status(), want)
		}

		time.Sleep(time.Millisecond)
	}
}

func TestLifecycleOrder(t *testing.T) {
	rec := &recorder{}
	a := newApp(t, Services(&fakeService{name: "a", rec: rec}, &fakeService{name: "b", rec: rec}))

	if a.Status() != StatusCreated {
		t.Fatalf("status after New = %v", a.Status())
	}

	res := launch(t, a, StatusRunning)

	if err := a.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	if err := <-res; err != nil {
		t.Fatalf("Launch() error = %v", err)
	}

	want := []string{"a.OnInit", "b.OnInit", "a.OnStart", "b.OnStart", "b.OnStop", "a.OnStop"}
	if got := rec.list(); !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}

	if a.Status() != StatusStopped {
		t.Errorf("status = %v, want Stopped", a.Status())
	}
}

func TestStartupFailureRollsBackInitializedServices(t *testing.T) {
	errFail := errors.New("fail")

	tests := []struct {
		name     string
		services func(*recorder) []Service
		want     []string
	}{
		{
			name: "OnInit fails",
			services: func(r *recorder) []Service {
				return []Service{
					&fakeService{name: "a", rec: r},
					&fakeService{name: "b", rec: r, initErr: errFail},
					&fakeService{name: "c", rec: r},
				}
			},
			// b did not complete OnInit, so it gets no OnStop.
			want: []string{"a.OnInit", "b.OnInit", "a.OnStop"},
		},
		{
			name: "OnStart fails",
			services: func(r *recorder) []Service {
				return []Service{
					&fakeService{name: "a", rec: r},
					&fakeService{name: "b", rec: r, startErr: errFail},
					&fakeService{name: "c", rec: r},
				}
			},
			// c completed OnInit but never started; it still gets OnStop.
			want: []string{"a.OnInit", "b.OnInit", "c.OnInit", "a.OnStart", "b.OnStart", "c.OnStop", "b.OnStop", "a.OnStop"},
		},
		{
			name: "OnInit panics",
			services: func(r *recorder) []Service {
				return []Service{&fakeService{name: "a", rec: r}, &fakeService{name: "b", rec: r, panicIn: "OnInit"}}
			},
			want: []string{"a.OnInit", "b.OnInit", "a.OnStop"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recorder{}
			a := newApp(t, Services(tt.services(rec)...))

			err := a.Launch()
			if err == nil {
				t.Fatal("Launch() error = nil, want error")
			}

			if !strings.Contains(err.Error(), `service "b"`) {
				t.Errorf("error %q does not name the failing service", err)
			}

			if got := rec.list(); !slices.Equal(got, tt.want) {
				t.Errorf("events = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPanicIsReportedAsError(t *testing.T) {
	a := newApp(t, Services(&fakeService{name: "a", rec: &recorder{}, panicIn: "OnStart"}))

	err := a.Launch()
	if !errors.Is(err, errPanic) || !strings.Contains(err.Error(), "boom") {
		t.Errorf("Launch() error = %v, want panic error with value", err)
	}
}

func TestStartupTimeout(t *testing.T) {
	rec := &recorder{}
	a := newApp(t, StartupTimeout(20*time.Millisecond),
		Services(&fakeService{name: "a", rec: rec, block: "OnStart"}))

	err := a.Launch()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Launch() error = %v, want DeadlineExceeded", err)
	}

	want := []string{"a.OnInit", "a.OnStart", "a.OnStop"}
	if got := rec.list(); !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
}

func TestShutdownTimeout(t *testing.T) {
	a := newApp(t, ShutdownTimeout(20*time.Millisecond),
		Services(&fakeService{name: "a", rec: &recorder{}, block: "OnStop"}))

	res := launch(t, a, StatusRunning)

	err := a.Shutdown(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Shutdown() error = %v, want DeadlineExceeded", err)
	}

	if err := <-res; !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Launch() error = %v, want DeadlineExceeded", err)
	}
}

func TestShutdownTimeoutBlamesOnlyTheSlowService(t *testing.T) {
	rec := &recorder{}
	a := newApp(t, ShutdownTimeout(20*time.Millisecond), Services(
		&fakeService{name: "a", rec: rec},
		&fakeService{name: "slow", rec: rec, block: "OnStop"},
	))

	res := launch(t, a, StatusRunning)
	_ = a.Shutdown(context.Background())

	err := <-res
	if err == nil || !strings.Contains(err.Error(), `service "slow"`) {
		t.Fatalf("Launch() error = %v, want an error for the slow service", err)
	}

	// a runs after the deadline passed in slow.OnStop; that timeout is not a's failure.
	if strings.Contains(err.Error(), `service "a"`) {
		t.Errorf("Launch() error = %v, blames a service that stopped cleanly", err)
	}

	if got := rec.list(); !slices.Contains(got, "a.OnStop") {
		t.Errorf("events = %v, want a.OnStop after the timeout", got)
	}
}

func TestPanicStackIsLoggedNotReturned(t *testing.T) {
	var buf bytes.Buffer

	a := newApp(t, LogWriter(&buf), Services(&fakeService{name: "a", rec: &recorder{}, panicIn: "OnInit"}))

	err := a.Launch()
	if strings.Contains(err.Error(), "goroutine") {
		t.Errorf("error contains a stack: %v", err)
	}

	if !strings.Contains(buf.String(), `"msg":"hook panicked"`) || !strings.Contains(buf.String(), `"stack":"goroutine`) {
		t.Errorf("log lacks the panic stack: %s", buf.String())
	}
}

func TestShutdownDuringStartupIsNotAnError(t *testing.T) {
	rec := &recorder{}
	a := newApp(t, Services(&fakeService{name: "a", rec: rec, block: "OnStart"}))

	res := launch(t, a, StatusStarting)

	if err := a.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}

	if err := <-res; err != nil {
		t.Errorf("Launch() error = %v, want nil after a requested stop", err)
	}

	if got := rec.list(); !slices.Contains(got, "a.OnStop") {
		t.Errorf("events = %v, want a.OnStop", got)
	}
}

func TestDrain(t *testing.T) {
	t.Run("waits DrainDelay in Draining", func(t *testing.T) {
		a := newApp(t, DrainDelay(100*time.Millisecond))
		res := launch(t, a, StatusRunning)

		go func() { _ = a.Shutdown(context.Background()) }()

		waitStatus(t, a, StatusDraining)

		if err := <-res; err != nil {
			t.Fatalf("Launch() error = %v", err)
		}
	})

	t.Run("Shutdown context ends drain early", func(t *testing.T) {
		a := newApp(t, DrainDelay(time.Hour))
		res := launch(t, a, StatusRunning)

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		if err := a.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Shutdown() error = %v, want DeadlineExceeded", err)
		}

		select {
		case err := <-res:
			if err != nil {
				t.Errorf("Launch() error = %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Launch did not return after the Shutdown context ended")
		}
	})
}

func TestLaunchIsOneShot(t *testing.T) {
	a := newApp(t)
	res := launch(t, a, StatusRunning)

	if err := a.Launch(); !errors.Is(err, ErrAlreadyLaunched) {
		t.Errorf("second Launch() error = %v, want ErrAlreadyLaunched", err)
	}

	if err := a.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	if err := <-res; err != nil {
		t.Fatalf("Launch() error = %v", err)
	}

	if err := a.Launch(); !errors.Is(err, ErrAlreadyLaunched) {
		t.Errorf("Launch() after stop error = %v, want ErrAlreadyLaunched", err)
	}

	// A second Shutdown after the stop returns at once with the same result.
	if err := a.Shutdown(context.Background()); err != nil {
		t.Errorf("second Shutdown() error = %v", err)
	}
}

func TestShutdownBeforeLaunch(t *testing.T) {
	a := newApp(t)

	if err := a.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	if a.Status() != StatusStopped {
		t.Errorf("status = %v, want Stopped", a.Status())
	}

	if err := a.Launch(); !errors.Is(err, ErrAlreadyLaunched) {
		t.Errorf("Launch() error = %v, want ErrAlreadyLaunched", err)
	}
}

func TestSignalStopsApp(t *testing.T) {
	rec := &recorder{}
	a := newApp(t, HandleSignals(true), Services(&fakeService{name: "a", rec: rec}))
	res := launch(t, a, StatusRunning)

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("kill: %v", err)
	}

	select {
	case err := <-res:
		if err != nil {
			t.Errorf("Launch() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Launch did not return after SIGTERM")
	}

	if got := rec.list(); !slices.Contains(got, "a.OnStop") {
		t.Errorf("events = %v, want a.OnStop", got)
	}
}

func TestDepsLogger(t *testing.T) {
	var buf bytes.Buffer

	s := &fakeService{name: "orders", rec: &recorder{}}
	a := newApp(t, LogWriter(&buf), Services(s))
	res := launch(t, a, StatusRunning)

	s.deps.Logger.Info("hello")

	if err := a.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := <-res; err != nil {
		t.Fatalf("Launch() error = %v", err)
	}

	for _, want := range []string{`"msg":"hello"`, `"service":"orders"`, `"app":"test-app"`, `"version":"1.0.0"`} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("log output lacks %s", want)
		}
	}
}

func TestRunExitCode(t *testing.T) {
	ok := newApp(t)
	if err := ok.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	// A stopped App fails Launch, which run reports as status 1.
	var stderr bytes.Buffer
	if code := run(ok, &stderr); code != 1 || !strings.Contains(stderr.String(), "already launched") {
		t.Errorf("run() = %d, stderr %q", code, stderr.String())
	}

	clean := newApp(t)
	go func() {
		waitStatus(t, clean, StatusRunning)
		_ = clean.Shutdown(context.Background())
	}()

	if code := run(clean, io.Discard); code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
}

func TestStatusString(t *testing.T) {
	for s := StatusCreated; s <= StatusStopped; s++ {
		if s.String() == "Unknown" {
			t.Errorf("Status(%d) has no name", s)
		}
	}

	if Status(99).String() != "Unknown" {
		t.Error("Status(99) should be Unknown")
	}
}
