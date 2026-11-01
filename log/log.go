// Package log builds the slog logger that go-svclib and its services use.
package log

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// Format is the output format of a logger.
type Format string

// Supported formats.
const (
	FormatJSON Format = "json"
	FormatText Format = "text"
)

var errUnknownFormat = errors.New("unknown log format")

// ParseFormat converts s to a Format. The match ignores case.
func ParseFormat(s string) (Format, error) {
	switch f := Format(strings.ToLower(s)); f {
	case FormatJSON, FormatText:
		return f, nil
	default:
		return "", fmt.Errorf("%w %q; use %q or %q", errUnknownFormat, s, FormatJSON, FormatText)
	}
}

// ParseLevel converts s (debug, info, warn, error, or a slog offset such as
// "info+2") to a slog.Level.
func ParseLevel(s string) (slog.Level, error) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return 0, fmt.Errorf("log level: %w", err)
	}

	return l, nil
}

// New returns a logger that writes to w in format f, filtered by level.
// The caller keeps level to change the level at run time.
func New(w io.Writer, f Format, level *slog.LevelVar) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}

	if f == FormatText {
		return slog.New(slog.NewTextHandler(w, opts))
	}

	return slog.New(slog.NewJSONHandler(w, opts))
}
