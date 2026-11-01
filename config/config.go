// Package config reads the env defaults for go-svclib.
//
// Options in code take precedence over these values. Every key has the
// prefix SVC_.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Prefix is the prefix of every env key that go-svclib reads.
const Prefix = "SVC_"

// Env holds the values that go-svclib reads from the environment.
type Env struct {
	Name    string `env:"NAME"`
	Version string `env:"VERSION"`
	Commit  string `env:"COMMIT"`

	LogLevel  string `env:"LOG_LEVEL"  envDefault:"info"`
	LogFormat string `env:"LOG_FORMAT" envDefault:"json"`

	StartupTimeout  time.Duration `env:"STARTUP_TIMEOUT"  envDefault:"30s"`
	DrainDelay      time.Duration `env:"DRAIN_DELAY"      envDefault:"5s"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"25s"`
	HookGracePeriod time.Duration `env:"HOOK_GRACE_PERIOD" envDefault:"2s"`

	HandleSignals bool `env:"HANDLE_SIGNALS" envDefault:"true"`
}

// Load reads Env from environ, a map of full key names (with Prefix) to values.
// A nil environ reads the process environment.
func Load(environ map[string]string) (Env, error) {
	var e Env
	if err := env.ParseWithOptions(&e, env.Options{Prefix: Prefix, Environment: environ}); err != nil {
		return Env{}, fmt.Errorf("config: %w", err)
	}

	return e, nil
}
