// Package tracker periodically checks the portal for new purchase orders,
// downloads their reports, converts them to the standard file and records
// them in the store for PAD. It only depends on interfaces, so it can be
// tested without a browser or Excel files.
package tracker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Khaym03/falatracker/internal/order"
)

// Session is a logged-in view of the portal used during one cycle.
type Session interface {
	Login(ctx context.Context) error
	ListOrders(ctx context.Context) ([]order.PurchaseOrder, error)
	// DownloadReports downloads the order's reports and returns their paths.
	DownloadReports(ctx context.Context, orderNumber string) (order.Files, error)
}

// OpenFunc opens a new Session at the start of each cycle.
type OpenFunc func(ctx context.Context) (Session, error)

// Converter turns the downloaded reports into the standard file. The
// returned record has the order number, billing date, files and issues.
type Converter interface {
	Convert(files order.Files) (order.Record, error)
}

// Store is the part of order.Store the tracker needs.
type Store interface {
	IsProcessed(orderNumber string) bool
	Save(r order.Record) error
}

// Tracker runs download cycles.
type Tracker struct {
	open  OpenFunc
	conv  Converter
	store Store
	log   *slog.Logger
	now   func() time.Time
}

// New returns a Tracker.
func New(open OpenFunc, conv Converter, store Store, log *slog.Logger) *Tracker {
	return &Tracker{open: open, conv: conv, store: store, log: log, now: time.Now}
}

// Start runs a cycle right away and then every interval, until ctx is done.
func (t *Tracker) Start(ctx context.Context, interval time.Duration) {
	t.log.Info("tracker started", "interval", interval)
	defer t.log.Info("tracker stopped")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		t.runCycle(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (t *Tracker) runCycle(ctx context.Context) {
	start := time.Now()
	t.log.Debug("cycle started")

	processed, err := t.RunOnce(ctx)
	if ctx.Err() != nil {
		return // shutting down; the error is just the cancellation
	}
	if err != nil {
		t.log.Error("cycle failed", "error", err, "processed", processed, "duration", time.Since(start))
		return
	}
	t.log.Info("cycle finished", "processed", processed, "duration", time.Since(start))
}

// RunOnce logs in and processes every pending order: download its reports,
// convert them and save the record (status ready). It returns how many
// orders were processed.
//
// An order whose download or conversion fails is not saved, so it is retried
// in the next cycle; within the current cycle it is skipped so that a broken
// order cannot block the others. A failure to save the record stops the
// cycle, because without the store the dedup rule cannot be trusted.
func (t *Tracker) RunOnce(ctx context.Context) (int, error) {
	s, err := t.open(ctx)
	if err != nil {
		return 0, fmt.Errorf("open session: %w", err)
	}
	if err := s.Login(ctx); err != nil {
		return 0, fmt.Errorf("login: %w", err)
	}

	attempted := make(map[string]bool)
	processed := 0

	for {
		// The list is read again after every download because the page is reloaded.
		orders, err := s.ListOrders(ctx)
		if err != nil {
			return processed, fmt.Errorf("list orders: %w", err)
		}

		next, ok := t.nextPending(orders, attempted)
		if !ok {
			return processed, nil
		}
		attempted[next.OrderNumber] = true

		log := t.log.With("order", next.OrderNumber)
		log.Info("processing order",
			"bu", next.BU,
			"status", next.Status,
			"creation_date", next.CreationDate,
			"total_amount", next.TotalAmount,
		)

		rec, err := t.process(ctx, s, next.OrderNumber)
		if err != nil {
			if ctx.Err() != nil {
				return processed, ctx.Err()
			}
			log.Error("order failed, will retry next cycle", "error", err)
			continue
		}

		if err := t.store.Save(rec); err != nil {
			return processed, fmt.Errorf("save order %s: %w", rec.OrderNumber, err)
		}
		processed++

		for _, issue := range rec.Issues {
			log.Warn("order issue", "issue", issue)
		}
		log.Info("order ready", "billing_date", rec.BillingDate, "standard", rec.Files.Standard)
	}
}

// process downloads and converts one order and returns its record.
func (t *Tracker) process(ctx context.Context, s Session, orderNumber string) (order.Record, error) {
	files, err := s.DownloadReports(ctx, orderNumber)
	if err != nil {
		return order.Record{}, fmt.Errorf("download: %w", err)
	}

	rec, err := t.conv.Convert(files)
	if err != nil {
		return order.Record{}, fmt.Errorf("convert: %w", err)
	}
	if rec.OrderNumber != orderNumber {
		return order.Record{}, fmt.Errorf("convert: report is for order %q", rec.OrderNumber)
	}

	rec.Status = order.StatusReady
	rec.ProcessedAt = t.now()
	return rec, nil
}

// nextPending returns the first order that still needs to be processed.
func (t *Tracker) nextPending(orders []order.PurchaseOrder, attempted map[string]bool) (order.PurchaseOrder, bool) {
	for _, o := range orders {
		if attempted[o.OrderNumber] || t.store.IsProcessed(o.OrderNumber) || !o.NeedsReports() {
			continue
		}
		return o, true
	}
	return order.PurchaseOrder{}, false
}
