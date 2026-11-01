package svc

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewValidation(t *testing.T) {
	rec := &recorder{}

	tests := []struct {
		name     string
		opts     []Option
		wantErrs []string
	}{
		{name: "valid", opts: baseOpts()},
		{name: "valid v-prefixed prerelease", opts: baseOpts(Version("v2.0.0-rc.1+build.5"))},
		{name: "bad name", opts: baseOpts(Name("Bad_Name")), wantErrs: []string{"name"}},
		{name: "bad version", opts: baseOpts(Version("1.0")), wantErrs: []string{"semantic version"}},
		{name: "no commit", opts: baseOpts(Commit("")), wantErrs: []string{"commit is required"}},
		{name: "bad log level", opts: baseOpts(LogLevel("loud")), wantErrs: []string{"log level"}},
		{name: "bad log format", opts: baseOpts(LogFormat("xml")), wantErrs: []string{"log format"}},
		{name: "nil writer", opts: baseOpts(LogWriter(nil)), wantErrs: []string{"log writer"}},
		{name: "zero startup timeout", opts: baseOpts(StartupTimeout(0)), wantErrs: []string{"startup timeout"}},
		{name: "negative drain", opts: baseOpts(DrainDelay(-time.Second)), wantErrs: []string{"drain delay"}},
		{name: "zero shutdown timeout", opts: baseOpts(ShutdownTimeout(0)), wantErrs: []string{"shutdown timeout"}},
		{name: "nil service", opts: baseOpts(Services(nil)), wantErrs: []string{"service 0 is nil"}},
		{name: "empty service name", opts: baseOpts(Services(&fakeService{rec: rec})), wantErrs: []string{"empty name"}},
		{
			name:     "duplicate service name",
			opts:     baseOpts(Services(&fakeService{name: "a", rec: rec}, &fakeService{name: "a", rec: rec})),
			wantErrs: []string{`"a" is used more than once`},
		},
		{
			name:     "all errors reported together",
			opts:     baseOpts(Name(""), Version(""), Commit("")),
			wantErrs: []string{"name", "semantic version", "commit is required"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.opts...)

			if len(tt.wantErrs) == 0 {
				if err != nil {
					t.Fatalf("New() error = %v", err)
				}

				return
			}

			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("New() error = %v, want ErrInvalidConfig", err)
			}

			for _, want := range tt.wantErrs {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q lacks %q", err, want)
				}
			}
		})
	}
}

func TestNewEnvDefaultsAndPrecedence(t *testing.T) {
	t.Setenv("SVC_NAME", "from-env")
	t.Setenv("SVC_VERSION", "3.0.0")
	t.Setenv("SVC_COMMIT", "envsha")
	t.Setenv("SVC_SHUTDOWN_TIMEOUT", "7s")
	t.Setenv("SVC_HANDLE_SIGNALS", "false")

	a, err := New(Version("4.0.0"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if a.opts.name != "from-env" || a.opts.commit != "envsha" || a.opts.shutdownTimeout != 7*time.Second || a.opts.handleSignals {
		t.Errorf("env values not applied: %+v", a.opts)
	}

	if a.opts.version != "4.0.0" {
		t.Errorf("version = %q, want the option to win over env", a.opts.version)
	}
}

func TestNewBadEnv(t *testing.T) {
	t.Setenv("SVC_STARTUP_TIMEOUT", "later")

	if _, err := New(baseOpts()...); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("New() error = %v, want ErrInvalidConfig", err)
	}
}
