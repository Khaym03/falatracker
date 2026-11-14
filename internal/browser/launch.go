// Package browser contains generic, site-agnostic helpers on top of go-rod:
// launching and closing the browser, retries, and a Page wrapper whose
// actions are bound to a context, have a timeout and never panic.
package browser

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/utils"
)

// ErrExecutableNotFound is returned when no Chrome/Chromium binary is installed.
var ErrExecutableNotFound = errors.New("browser executable not found")

// Options configures the launched browser.
type Options struct {
	Headless    bool
	UserDataDir string
	DebugPort   int // 0 picks a random port
	Trace       bool
}

// Launch starts a local browser and connects to it. Rod trace output is sent
// to log at debug level.
func Launch(opts Options, log *slog.Logger) (*rod.Browser, error) {
	bin, ok := launcher.LookPath()
	if !ok {
		return nil, ErrExecutableNotFound
	}

	log.Info("launching browser",
		"bin", bin,
		"headless", opts.Headless,
		"user_data_dir", opts.UserDataDir,
		"debug_port", opts.DebugPort,
	)

	l := launcher.New().
		Bin(bin).
		Leakless(false).
		Headless(opts.Headless).
		UserDataDir(opts.UserDataDir)
	if opts.DebugPort > 0 {
		l = l.RemoteDebuggingPort(opts.DebugPort)
	}

	controlURL, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("launch browser: %w", err)
	}

	b := rod.New().
		ControlURL(controlURL).
		Trace(opts.Trace).
		Logger(utils.Log(func(args ...any) {
			log.Debug(strings.TrimSpace(fmt.Sprint(args...)), "component", "rod")
		}))

	if err := b.Connect(); err != nil {
		l.Kill()
		return nil, fmt.Errorf("connect to browser: %w", err)
	}
	return b, nil
}

// Close closes the browser, giving up when ctx is done.
func Close(ctx context.Context, b *rod.Browser) error {
	errCh := make(chan error, 1)
	go func() { errCh <- b.Close() }()

	select {
	case <-ctx.Done():
		return fmt.Errorf("close browser: %w", ctx.Err())
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("close browser: %w", err)
		}
		return nil
	}
}
