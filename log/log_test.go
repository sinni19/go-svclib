package log

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestParseFormat(t *testing.T) {
	tests := []struct {
		in      string
		want    Format
		wantErr bool
	}{
		{in: "json", want: FormatJSON},
		{in: "TEXT", want: FormatText},
		{in: "xml", wantErr: true},
		{in: "", wantErr: true},
	}

	for _, tt := range tests {
		got, err := ParseFormat(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("ParseFormat(%q) = %q, %v; want %q, wantErr %v", tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in      string
		want    slog.Level
		wantErr bool
	}{
		{in: "debug", want: slog.LevelDebug},
		{in: "INFO", want: slog.LevelInfo},
		{in: "warn", want: slog.LevelWarn},
		{in: "error", want: slog.LevelError},
		{in: "info+2", want: slog.LevelInfo + 2},
		{in: "verbose", wantErr: true},
	}

	for _, tt := range tests {
		got, err := ParseLevel(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("ParseLevel(%q) = %v, %v; want %v, wantErr %v", tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestNew(t *testing.T) {
	var buf bytes.Buffer

	level := new(slog.LevelVar)
	level.Set(slog.LevelWarn)

	l := New(&buf, FormatJSON, level)
	l.Info("dropped")
	l.Warn("kept", "k", "v")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output is not one JSON record: %v: %q", err, buf.String())
	}

	if rec["msg"] != "kept" || rec["k"] != "v" {
		t.Errorf("record = %v", rec)
	}

	// The level is shared, so a change applies to a logger that already exists.
	buf.Reset()
	level.Set(slog.LevelDebug)
	l.Debug("now kept")

	if !strings.Contains(buf.String(), "now kept") {
		t.Errorf("debug record missing after level change: %q", buf.String())
	}

	buf.Reset()
	New(&buf, FormatText, level).Info("hello")

	if !strings.Contains(buf.String(), "msg=hello") {
		t.Errorf("text output = %q", buf.String())
	}
}
