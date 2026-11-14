package order

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func record(n string, s Status) Record {
	return Record{OrderNumber: n, Status: s, BillingDate: "2026-11-03", ProcessedAt: time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)}
}

// storeContract runs the behaviour every Store must have.
func storeContract(t *testing.T, newStore func(t *testing.T) Store) {
	t.Run("save and query", func(t *testing.T) {
		s := newStore(t)
		if s.IsProcessed("1") {
			t.Fatal("new store must be empty")
		}
		if err := s.Save(record("1", StatusReady)); err != nil {
			t.Fatalf("Save: %v", err)
		}
		if !s.IsProcessed("1") || s.IsProcessed("2") {
			t.Error("IsProcessed does not reflect the saved order")
		}
	})

	t.Run("save rejects an empty order number", func(t *testing.T) {
		if err := newStore(t).Save(Record{}); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("list filters by status and sorts", func(t *testing.T) {
		s := newStore(t)
		for _, r := range []Record{record("3", StatusReady), record("1", StatusLoaded), record("2", StatusReady)} {
			if err := s.Save(r); err != nil {
				t.Fatal(err)
			}
		}
		if got := numbers(s.List(StatusReady)); got != "[2 3]" {
			t.Errorf("List(ready) = %s, want [2 3]", got)
		}
		if got := numbers(s.List("")); got != "[1 2 3]" {
			t.Errorf("List(all) = %s, want [1 2 3]", got)
		}
	})

	t.Run("mark loaded", func(t *testing.T) {
		s := newStore(t)
		if err := s.Save(record("1", StatusReady)); err != nil {
			t.Fatal(err)
		}
		at := time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC)
		r, err := s.MarkLoaded("1", at)
		if err != nil {
			t.Fatalf("MarkLoaded: %v", err)
		}
		if r.Status != StatusLoaded || r.LoadedAt == nil || !r.LoadedAt.Equal(at) {
			t.Errorf("record = %+v", r)
		}
		if got := s.List(StatusLoaded); len(got) != 1 {
			t.Errorf("List(loaded) = %v", got)
		}
		// A loaded order still counts as processed: it is never downloaded again.
		if !s.IsProcessed("1") {
			t.Error("loaded order must stay processed")
		}
	})

	t.Run("mark loaded unknown order", func(t *testing.T) {
		_, err := newStore(t).MarkLoaded("404", time.Now())
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("error = %v, want ErrNotFound", err)
		}
	})
}

func numbers(rs []Record) string {
	ns := make([]string, len(rs))
	for i, r := range rs {
		ns[i] = r.OrderNumber
	}
	return fmt.Sprint(ns)
}

func TestMemoryStore(t *testing.T) {
	storeContract(t, func(*testing.T) Store { return NewMemoryStore() })
}

func TestMemoryStoreConcurrent(t *testing.T) {
	s := NewMemoryStore()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n := fmt.Sprint(i % 10)
			_ = s.Save(record(n, StatusReady))
			_ = s.IsProcessed(n)
			_ = s.List("")
		}()
	}
	wg.Wait()
}

func TestStatusValid(t *testing.T) {
	if !StatusReady.Valid() || !StatusLoaded.Valid() || Status("other").Valid() || Status("").Valid() {
		t.Error("unexpected Valid result")
	}
}
