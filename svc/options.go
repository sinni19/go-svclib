package svc

import (
	"io"
	"time"
)

// Option configures an App. Options take precedence over SVC_* env values.
type Option func(*options)

type options struct {
	name    string
	version string
	commit  string

	logLevel  string
	logFormat string
	logWriter io.Writer

	startupTimeout  time.Duration
	drainDelay      time.Duration
	shutdownTimeout time.Duration
	handleSignals   bool

	services []Service
}

// Name sets the application name (env SVC_NAME). Required.
// It must match ^[a-z][a-z0-9-]{0,62}$ because it becomes a metric and log label.
func Name(name string) Option { return func(o *options) { o.name = name } }

// Version sets the semantic version, for example "1.4.0" or "v1.4.0-rc.1"
// (env SVC_VERSION). Required.
func Version(version string) Option { return func(o *options) { o.version = version } }

// Commit sets the source commit ID (env SVC_COMMIT). Required.
func Commit(commit string) Option { return func(o *options) { o.commit = commit } }

// LogLevel sets the minimum log level: debug, info, warn, or error
// (env SVC_LOG_LEVEL, default info).
func LogLevel(level string) Option { return func(o *options) { o.logLevel = level } }

// LogFormat sets the log format: json or text (env SVC_LOG_FORMAT, default json).
func LogFormat(format string) Option { return func(o *options) { o.logFormat = format } }

// LogWriter sets the log destination. The default is os.Stdout.
func LogWriter(w io.Writer) Option { return func(o *options) { o.logWriter = w } }

// StartupTimeout limits OnInit and OnStart of all services together
// (env SVC_STARTUP_TIMEOUT, default 30s).
func StartupTimeout(d time.Duration) Option { return func(o *options) { o.startupTimeout = d } }

// DrainDelay is the wait between the stop request and OnStop, so that load
// balancers see the App as not ready (env SVC_DRAIN_DELAY, default 5s).
func DrainDelay(d time.Duration) Option { return func(o *options) { o.drainDelay = d } }

// ShutdownTimeout limits OnStop of all services together
// (env SVC_SHUTDOWN_TIMEOUT, default 25s).
func ShutdownTimeout(d time.Duration) Option { return func(o *options) { o.shutdownTimeout = d } }

// HandleSignals controls whether Launch stops the App on SIGINT and SIGTERM
// (env SVC_HANDLE_SIGNALS, default true).
func HandleSignals(enabled bool) Option { return func(o *options) { o.handleSignals = enabled } }

// Services adds services to the App. Hooks run in the order given; OnStop runs
// in reverse order.
func Services(s ...Service) Option {
	return func(o *options) { o.services = append(o.services, s...) }
}
