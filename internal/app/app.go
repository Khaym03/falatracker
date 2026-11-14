// Package app wires the components together and manages their lifecycle.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/Khaym03/falatracker/internal/browser"
	"github.com/Khaym03/falatracker/internal/falabella"
	"github.com/Khaym03/falatracker/internal/order"
	"github.com/Khaym03/falatracker/internal/portal"
	"github.com/Khaym03/falatracker/internal/server"
	"github.com/Khaym03/falatracker/internal/tracker"
)

// Config is everything Run needs.
type Config struct {
	Interval        time.Duration
	HTTPAddr        string
	ShutdownTimeout time.Duration
	Browser         browser.Options
	Credential      portal.Credential

	// DataDir holds the store (orders.json) and one folder per order with
	// its downloaded reports and the standard file (orders/<number>/).
	DataDir string

	// Converter turns the reports into the standard file.
	Converter *falabella.Converter
}

// StorePath returns the path of the order store inside dataDir.
func StorePath(dataDir string) string { return filepath.Join(dataDir, "orders.json") }

// OrdersDir returns the folder of the per-order files inside dataDir.
func OrdersDir(dataDir string) string { return filepath.Join(dataDir, "orders") }

// Run starts the HTTP server, the browser and the tracker, and blocks until
// ctx is cancelled or a shutdown is requested over HTTP.
func Run(ctx context.Context, cfg Config, log *slog.Logger) error {
	ctx, stop := context.WithCancel(ctx)
	defer stop()

	store, err := order.OpenFileStore(StorePath(cfg.DataDir))
	if err != nil {
		return err
	}
	log.Info("order store opened", "path", StorePath(cfg.DataDir), "orders", len(store.List("")))

	srv := server.New(cfg.HTTPAddr, store, stop, log)
	if err := srv.Start(); err != nil {
		return err
	}

	b, err := browser.Launch(cfg.Browser, log)
	if err != nil {
		return errors.Join(err, shutdownServer(srv, cfg.ShutdownTimeout))
	}

	client := portal.New(b, cfg.Credential, OrdersDir(cfg.DataDir), log)
	open := func(ctx context.Context) (tracker.Session, error) { return client.Open(ctx) }
	t := tracker.New(open, cfg.Converter, store, log)

	trackerDone := make(chan struct{})
	go func() {
		defer close(trackerDone)
		t.Start(ctx, cfg.Interval)
	}()

	<-ctx.Done()
	log.Info("shutting down", "timeout", cfg.ShutdownTimeout)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	var errs []error
	if err := srv.Shutdown(shutdownCtx); err != nil {
		errs = append(errs, err)
	}
	select {
	case <-trackerDone:
	case <-shutdownCtx.Done():
		errs = append(errs, fmt.Errorf("tracker did not stop in time: %w", shutdownCtx.Err()))
	}
	if err := browser.Close(shutdownCtx, b); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func shutdownServer(srv *server.Server, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return srv.Shutdown(ctx)
}
