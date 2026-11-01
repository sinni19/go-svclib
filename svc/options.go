package svc

import (
	"crypto/tls"
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
	hookGracePeriod time.Duration
	handleSignals   bool

	addr              string
	tls               *tls.Config
	insecure          bool
	readHeaderTimeout time.Duration
	idleTimeout       time.Duration
	maxMessageBytes   int
	rpcDefaultTimeout time.Duration
	rpcMaxTimeout     time.Duration

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

// HookGracePeriod is the budget of a hook that starts after its phase timeout
// already passed, for example an OnStop that follows an abandoned OnStop
// (env SVC_HOOK_GRACE_PERIOD, default 2s). It lets each service release its
// resources while shutdown time stays bounded.
func HookGracePeriod(d time.Duration) Option { return func(o *options) { o.hookGracePeriod = d } }

// HandleSignals controls whether Launch stops the App on SIGINT and SIGTERM
// (env SVC_HANDLE_SIGNALS, default true).
func HandleSignals(enabled bool) Option { return func(o *options) { o.handleSignals = enabled } }

// Addr sets the listen address of the RPC server (env SVC_ADDR, default
// ":8443"). The RPC server runs only when a service has handlers.
func Addr(addr string) Option { return func(o *options) { o.addr = addr } }

// TLS sets the TLS config of the RPC server. The config must have a
// certificate source. The App uses a clone with TLS 1.2 or later.
// One of TLS and Insecure is required.
func TLS(cfg *tls.Config) Option { return func(o *options) { o.tls = cfg } }

// Insecure makes the RPC server serve h2c and HTTP/1.1 without TLS. Use it
// only for local development and tests, or behind a proxy that terminates
// TLS on the same host. One of TLS and Insecure is required.
func Insecure() Option { return func(o *options) { o.insecure = true } }

// ReadHeaderTimeout limits the time to read the request headers
// (env SVC_READ_HEADER_TIMEOUT, default 5s).
func ReadHeaderTimeout(d time.Duration) Option { return func(o *options) { o.readHeaderTimeout = d } }

// IdleTimeout closes keep-alive connections that are idle for d
// (env SVC_IDLE_TIMEOUT, default 120s).
func IdleTimeout(d time.Duration) Option { return func(o *options) { o.idleTimeout = d } }

// MaxMessageBytes limits the size of one request message
// (env SVC_MAX_MESSAGE_BYTES, default 4 MiB).
func MaxMessageBytes(n int) Option { return func(o *options) { o.maxMessageBytes = n } }

// RPCDefaultTimeout is the deadline of a unary call whose client sets none
// (env SVC_RPC_DEFAULT_TIMEOUT, default 30s).
func RPCDefaultTimeout(d time.Duration) Option { return func(o *options) { o.rpcDefaultTimeout = d } }

// RPCMaxTimeout caps the deadline that a client sets on a unary call
// (env SVC_RPC_MAX_TIMEOUT, default 60s). Streams have no cap.
func RPCMaxTimeout(d time.Duration) Option { return func(o *options) { o.rpcMaxTimeout = d } }

// Services adds services to the App. Hooks run in the order given; OnStop runs
// in reverse order.
func Services(s ...Service) Option {
	return func(o *options) { o.services = append(o.services, s...) }
}
