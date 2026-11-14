// Package logging builds the application's slog logger: human-readable text
// in development, JSON in production, written to the console and to a file.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Supported environments.
const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
)

// Options configures New.
type Options struct {
	Env      string     // EnvDevelopment or EnvProduction
	Level    slog.Level // minimum level to log
	FilePath string     // log file (appended to); empty disables file output
	Console  io.Writer  // console output; defaults to os.Stderr
}

// New returns a logger and a function that closes the log file.
func New(opts Options) (*slog.Logger, func() error, error) {
	if err := ValidateEnv(opts.Env); err != nil {
		return nil, nil, err
	}

	out := opts.Console
	if out == nil {
		out = os.Stderr
	}

	closeFn := func() error { return nil }
	if opts.FilePath != "" {
		f, err := openFile(opts.FilePath)
		if err != nil {
			return nil, nil, err
		}
		out = io.MultiWriter(out, f)
		closeFn = f.Close
	}

	handlerOpts := &slog.HandlerOptions{Level: opts.Level}

	var h slog.Handler
	if opts.Env == EnvProduction {
		h = slog.NewJSONHandler(out, handlerOpts)
	} else {
		h = slog.NewTextHandler(out, handlerOpts)
	}
	return slog.New(h), closeFn, nil
}

// ValidateEnv checks that env is a supported environment.
func ValidateEnv(env string) error {
	switch env {
	case EnvDevelopment, EnvProduction:
		return nil
	default:
		return fmt.Errorf("unknown env %q: use %q or %q", env, EnvDevelopment, EnvProduction)
	}
}

// DefaultLevel is debug in development and info in production.
func DefaultLevel(env string) slog.Level {
	if env == EnvProduction {
		return slog.LevelInfo
	}
	return slog.LevelDebug
}

// ParseLevel parses "debug", "info", "warn" or "error". An empty string
// returns the default level for env.
func ParseLevel(s, env string) (slog.Level, error) {
	if s == "" {
		return DefaultLevel(env), nil
	}
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return 0, fmt.Errorf("invalid log level %q: %w", s, err)
	}
	return l, nil
}

func openFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	return f, nil
}
