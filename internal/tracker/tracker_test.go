package tracker

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Khaym03/falatracker/internal/order"
)

// fakeSession is an in-memory Session.
type fakeSession struct {
	orders    []order.PurchaseOrder
	loginErr  error
	listErr   error
	failOn    map[string]bool // order numbers whose download fails
	downloads []string        // order numbers passed to DownloadReports, in order
	lists     int
}

func (f *fakeSession) Login(context.Context) error { return f.loginErr }

func (f *fakeSession) ListOrders(context.Context) ([]order.PurchaseOrder, error) {
	f.lists++
	return f.orders, f.listErr
}

func (f *fakeSession) DownloadReports(_ context.Context, n string) (order.Files, error) {
	f.downloads = append(f.downloads, n)
	if f.failOn[n] {
		return order.Files{}, errors.New("download failed")
	}
	return order.Files{Products: n + "-products.xlsx", Distribution: n + "-distribution.xlsx"}, nil
}

// fakeConverter "converts" by deriving the record from the file names.
type fakeConverter struct {
	failOn   map[string]bool
	wrongFor map[string]bool // returns a record for another order
}

func (c *fakeConverter) Convert(files order.Files) (order.Record, error) {
	n := files.Products[:len(files.Products)-len("-products.xlsx")]
	if c.failOn[n] {
		return order.Record{}, errors.New("bad excel")
	}
	if c.wrongFor[n] {
		n = "other"
	}
	files.Standard = n + "-standard.xlsx"
	return order.Record{OrderNumber: n, BillingDate: "2026-11-03", Files: files, Issues: []string{"issue"}}, nil
}

// failingStore fails every Save.
type failingStore struct{ *order.MemoryStore }

func (failingStore) Save(order.Record) error { return errors.New("disk full") }

var fixedNow = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

func newTracker(s *fakeSession, conv *fakeConverter, store Store) *Tracker {
	t := New(func(context.Context) (Session, error) { return s, nil }, conv, store, discard())
	t.now = func() time.Time { return fixedNow }
	return t
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func pending(n string) order.PurchaseOrder {
	return order.PurchaseOrder{OrderNumber: n, Status: "Pending"}
}

func TestRunOnceProcessesOnlyPendingOrders(t *testing.T) {
	store := order.NewMemoryStore()
	_ = store.Save(order.Record{OrderNumber: "already", Status: order.StatusLoaded})

	s := &fakeSession{orders: []order.PurchaseOrder{
		pending("1"),
		{OrderNumber: "canceled", Status: "Canceled"},
		{OrderNumber: "agreed", Status: "Pending", AgreementPercentage: 50},
		pending("already"),
		pending("2"),
	}}

	n, err := newTracker(s, &fakeConverter{}, store).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 2 {
		t.Errorf("processed = %d, want 2", n)
	}
	if want := []string{"1", "2"}; !slices.Equal(s.downloads, want) {
		t.Errorf("downloads = %v, want %v", s.downloads, want)
	}
	if store.IsProcessed("canceled") || store.IsProcessed("agreed") {
		t.Error("skipped orders must not be stored")
	}
	// One list per processed order plus the final one that finds nothing left.
	if s.lists != 3 {
		t.Errorf("lists = %d, want 3", s.lists)
	}
}

func TestRunOnceSavesTheConvertedRecord(t *testing.T) {
	store := order.NewMemoryStore()
	s := &fakeSession{orders: []order.PurchaseOrder{pending("1")}}

	if _, err := newTracker(s, &fakeConverter{}, store).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	recs := store.List(order.StatusReady)
	if len(recs) != 1 {
		t.Fatalf("ready records = %d, want 1", len(recs))
	}
	r := recs[0]
	if r.OrderNumber != "1" || r.Status != order.StatusReady || !r.ProcessedAt.Equal(fixedNow) {
		t.Errorf("record = %+v", r)
	}
	if r.Files.Products != "1-products.xlsx" || r.Files.Standard != "1-standard.xlsx" || r.BillingDate != "2026-11-03" {
		t.Errorf("record files/billing = %+v", r)
	}
}

func TestRunOnceFailuresAreRetriedNextCycle(t *testing.T) {
	tests := []struct {
		name string
		s    *fakeSession
		conv *fakeConverter
	}{
		{"download fails", &fakeSession{failOn: map[string]bool{"bad": true}}, &fakeConverter{}},
		{"conversion fails", &fakeSession{}, &fakeConverter{failOn: map[string]bool{"bad": true}}},
		{"report of another order", &fakeSession{}, &fakeConverter{wrongFor: map[string]bool{"bad": true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := order.NewMemoryStore()
			tt.s.orders = []order.PurchaseOrder{pending("bad"), pending("good")}
			tr := newTracker(tt.s, tt.conv, store)

			n, err := tr.RunOnce(context.Background())
			if err != nil {
				t.Fatalf("RunOnce: %v", err)
			}
			// The broken order is tried once and does not block the others.
			if n != 1 || !slices.Equal(tt.s.downloads, []string{"bad", "good"}) {
				t.Errorf("processed = %d, downloads = %v", n, tt.s.downloads)
			}
			if store.IsProcessed("bad") {
				t.Error("failed order must not be stored")
			}

			// Next cycle it is attempted again; the good one is not.
			tt.s.downloads = nil
			if _, err := tr.RunOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(tt.s.downloads, []string{"bad"}) {
				t.Errorf("second cycle downloads = %v, want [bad]", tt.s.downloads)
			}
		})
	}
}

func TestRunOnceStopsWhenTheStoreFails(t *testing.T) {
	s := &fakeSession{orders: []order.PurchaseOrder{pending("1"), pending("2")}}
	store := failingStore{order.NewMemoryStore()}

	_, err := newTracker(s, &fakeConverter{}, store).RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if len(s.downloads) != 1 {
		t.Errorf("downloads = %v, the cycle should stop after the first save error", s.downloads)
	}
}

func TestRunOnceErrors(t *testing.T) {
	errBoom := errors.New("boom")

	tests := []struct {
		name string
		open OpenFunc
	}{
		{"open", func(context.Context) (Session, error) { return nil, errBoom }},
		{"login", func(context.Context) (Session, error) { return &fakeSession{loginErr: errBoom}, nil }},
		{"list", func(context.Context) (Session, error) { return &fakeSession{listErr: errBoom}, nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.open, &fakeConverter{}, order.NewMemoryStore(), discard()).RunOnce(context.Background())
			if !errors.Is(err, errBoom) {
				t.Fatalf("RunOnce error = %v, want %v", err, errBoom)
			}
		})
	}
}

func TestRunOnceStopsWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &cancellingSession{fakeSession: fakeSession{orders: []order.PurchaseOrder{pending("1"), pending("2")}}, cancel: cancel}

	open := func(context.Context) (Session, error) { return s, nil }
	_, err := New(open, &fakeConverter{}, order.NewMemoryStore(), discard()).RunOnce(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunOnce error = %v, want context.Canceled", err)
	}
	if len(s.downloads) != 1 {
		t.Errorf("downloads = %v, want only the first one", s.downloads)
	}
}

// cancellingSession cancels the context during the first download and fails it.
type cancellingSession struct {
	fakeSession
	cancel context.CancelFunc
}

func (c *cancellingSession) DownloadReports(ctx context.Context, n string) (order.Files, error) {
	c.downloads = append(c.downloads, n)
	c.cancel()
	return order.Files{}, ctx.Err()
}

func TestStartRunsImmediatelyAndOnEveryTick(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var cycles atomic.Int32
	open := func(context.Context) (Session, error) {
		if cycles.Add(1) == 3 {
			cancel()
		}
		return &fakeSession{}, nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		New(open, &fakeConverter{}, order.NewMemoryStore(), discard()).Start(ctx, 10*time.Millisecond)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after the context was cancelled")
	}
	if got := cycles.Load(); got != 3 {
		t.Errorf("cycles = %d, want 3", got)
	}
}

func TestStartKeepsRunningAfterAFailedCycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var cycles atomic.Int32
	open := func(context.Context) (Session, error) {
		if cycles.Add(1) == 2 {
			cancel()
		}
		return nil, errors.New("browser crashed")
	}

	New(open, &fakeConverter{}, order.NewMemoryStore(), discard()).Start(ctx, 10*time.Millisecond)

	if got := cycles.Load(); got != 2 {
		t.Errorf("cycles = %d, want 2", got)
	}
}
