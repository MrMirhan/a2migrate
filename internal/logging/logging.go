// Package logging configures a structured slog logger used across a2migrate.
//
// The logger writes to stderr by default so it does not contaminate stdout
// (which may be parsed by callers). Verbosity is controlled by the global
// --verbose flag and defaults to INFO.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Level represents log severity in increasing order of verbosity reduction.
type Level slog.Level

const (
	LevelError Level = Level(slog.LevelError)
	LevelWarn  Level = Level(slog.LevelWarn)
	LevelInfo  Level = Level(slog.LevelInfo)
	LevelDebug Level = Level(slog.LevelDebug)
)

// ParseLevel converts a string ("error"|"warn"|"info"|"debug") to a Level.
// Unknown values fall back to INFO.
func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "error":
		return LevelError
	case "warn", "warning":
		return LevelWarn
	case "debug":
		return LevelDebug
	default:
		return LevelInfo
	}
}

// Options configures the logger.
type Options struct {
	Level  Level
	Format string // "text" or "json"
	Writer io.Writer
}

// Setup configures the default slog logger and returns it.
// Calling Setup multiple times replaces the previous default.
func Setup(opts Options) *slog.Logger {
	w := opts.Writer
	if w == nil {
		w = os.Stderr
	}
	handlerOpts := &slog.HandlerOptions{Level: slog.Level(opts.Level)}

	var h slog.Handler
	switch strings.ToLower(strings.TrimSpace(opts.Format)) {
	case "json":
		h = slog.NewJSONHandler(w, handlerOpts)
	default:
		h = slog.NewTextHandler(w, handlerOpts)
	}

	logger := slog.New(h)
	slog.SetDefault(logger)
	return logger
}
