// Package cli defines the falatracker command line. It only parses flags and
// environment into an app.Config; the actual work happens in package app.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"github.com/Khaym03/falatracker/internal/app"
	"github.com/Khaym03/falatracker/internal/browser"
	"github.com/Khaym03/falatracker/internal/logging"
	"github.com/Khaym03/falatracker/internal/portal"
	"github.com/Khaym03/falatracker/internal/version"
)

// Environment variables holding the portal credentials.
const (
	envUser     = "USER"
	envPassword = "PASSWORD"
)

// runFunc runs the application. It is app.Run in production and a fake in tests.
type runFunc func(ctx context.Context, cfg app.Config, log *slog.Logger) error

type options struct {
	env             string
	logLevel        string
	logFile         string
	envFile         string
	interval        time.Duration
	addr            string
	shutdownTimeout time.Duration
	headless        bool
	userDataDir     string
	debugPort       int
	trace           bool
	dataDir         string
	conversion      conversionOptions
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := newRootCmd(app.Run).ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	return 0
}

func newRootCmd(run runFunc) *cobra.Command {
	var opts options

	cmd := &cobra.Command{
		Use:           "falatracker",
		Short:         "Monitor Falabella Business Center purchase orders and download their reports",
		Version:       version.Get().String(),
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return opts.run(cmd.Context(), run)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.env, "env", logging.EnvDevelopment, "environment: development (text logs) or production (JSON logs)")
	f.StringVar(&opts.logLevel, "log-level", "", "debug, info, warn or error (default: debug in development, info in production)")
	f.StringVar(&opts.logFile, "log-file", "logs/falatracker.log", "file where logs are appended; empty disables it")
	f.StringVar(&opts.envFile, "env-file", ".env", "optional .env file with "+envUser+" and "+envPassword)
	f.DurationVar(&opts.interval, "interval", 3*time.Minute, "time between checks")
	f.StringVar(&opts.addr, "addr", ":2027", "address of the HTTP control server")
	f.DurationVar(&opts.shutdownTimeout, "shutdown-timeout", 5*time.Second, "maximum time allowed for a graceful shutdown")
	f.BoolVar(&opts.headless, "headless", false, "run the browser without a window")
	f.StringVar(&opts.userDataDir, "user-data-dir", "./rod_profile", "browser profile directory (keeps the session between runs)")
	f.IntVar(&opts.debugPort, "debug-port", 9222, "browser remote debugging port; 0 picks a free one")
	f.BoolVar(&opts.trace, "trace", true, "trace browser actions (logged at debug level)")
	f.StringVar(&opts.dataDir, "data-dir", "data", "folder for the order store and the per-order files")
	addConversionFlags(f, &opts.conversion)

	cmd.AddCommand(newVersionCmd(), newConvertCmd())
	return cmd
}

func (o options) run(ctx context.Context, run runFunc) error {
	if err := o.validate(); err != nil {
		return err
	}
	level, err := logging.ParseLevel(o.logLevel, o.env)
	if err != nil {
		return err
	}

	log, closeLog, err := logging.New(logging.Options{Env: o.env, Level: level, FilePath: o.logFile})
	if err != nil {
		return err
	}
	defer func() { _ = closeLog() }()

	info := version.Get()
	log.Info("starting falatracker", "version", info.Version, "commit", info.Commit, "env", o.env)

	cfg, err := o.config(log)
	if err != nil {
		log.Error("invalid configuration", "error", err)
		return err
	}

	if err := run(ctx, cfg, log); err != nil {
		log.Error("falatracker stopped with error", "error", err)
		return err
	}
	log.Info("falatracker stopped")
	return nil
}

func (o options) validate() error {
	if err := logging.ValidateEnv(o.env); err != nil {
		return err
	}
	if o.interval <= 0 {
		return errors.New("--interval must be greater than zero")
	}
	if o.shutdownTimeout <= 0 {
		return errors.New("--shutdown-timeout must be greater than zero")
	}
	return nil
}

// config loads the template, the .env file and credentials and builds the
// app config.
func (o options) config(log *slog.Logger) (app.Config, error) {
	conv, err := o.conversion.converter()
	if err != nil {
		return app.Config{}, err
	}

	if err := loadEnvFile(o.envFile, log); err != nil {
		return app.Config{}, err
	}

	cred := portal.Credential{User: os.Getenv(envUser), Password: os.Getenv(envPassword)}
	if err := cred.Validate(); err != nil {
		return app.Config{}, fmt.Errorf("credentials from %s/%s: %w", envUser, envPassword, err)
	}

	return app.Config{
		Interval:        o.interval,
		HTTPAddr:        o.addr,
		ShutdownTimeout: o.shutdownTimeout,
		Browser: browser.Options{
			Headless:    o.headless,
			UserDataDir: o.userDataDir,
			DebugPort:   o.debugPort,
			Trace:       o.trace,
		},
		Credential: cred,
		DataDir:    o.dataDir,
		Converter:  conv,
	}, nil
}

// loadEnvFile loads path into the environment. A missing file is not an
// error: in production the variables can come from the real environment.
func loadEnvFile(path string, log *slog.Logger) error {
	if path == "" {
		return nil
	}
	err := godotenv.Load(path)
	switch {
	case err == nil:
		log.Debug("env file loaded", "path", path)
		return nil
	case errors.Is(err, fs.ErrNotExist):
		log.Debug("env file not found, using process environment", "path", path)
		return nil
	default:
		return fmt.Errorf("load env file %s: %w", path, err)
	}
}
