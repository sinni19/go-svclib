package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		environ map[string]string
		want    Env
		wantErr bool
	}{
		{
			name:    "defaults",
			environ: map[string]string{},
			want: Env{
				LogLevel: "info", LogFormat: "json",
				StartupTimeout: 30 * time.Second, DrainDelay: 5 * time.Second, ShutdownTimeout: 25 * time.Second,
				HandleSignals: true,
			},
		},
		{
			name: "all set",
			environ: map[string]string{
				"SVC_NAME": "orders", "SVC_VERSION": "1.2.3", "SVC_COMMIT": "abc",
				"SVC_LOG_LEVEL": "debug", "SVC_LOG_FORMAT": "text",
				"SVC_STARTUP_TIMEOUT": "1m", "SVC_DRAIN_DELAY": "0s", "SVC_SHUTDOWN_TIMEOUT": "10s",
				"SVC_HANDLE_SIGNALS": "false",
			},
			want: Env{
				Name: "orders", Version: "1.2.3", Commit: "abc",
				LogLevel: "debug", LogFormat: "text",
				StartupTimeout: time.Minute, DrainDelay: 0, ShutdownTimeout: 10 * time.Second,
				HandleSignals: false,
			},
		},
		{
			name:    "unprefixed key is ignored",
			environ: map[string]string{"NAME": "orders"},
			want: Env{
				LogLevel: "info", LogFormat: "json",
				StartupTimeout: 30 * time.Second, DrainDelay: 5 * time.Second, ShutdownTimeout: 25 * time.Second,
				HandleSignals: true,
			},
		},
		{name: "bad duration", environ: map[string]string{"SVC_DRAIN_DELAY": "soon"}, wantErr: true},
		{name: "bad bool", environ: map[string]string{"SVC_HANDLE_SIGNALS": "maybe"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Load(tt.environ)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && got != tt.want {
				t.Errorf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
