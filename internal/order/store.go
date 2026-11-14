package order

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ErrNotFound is returned when an order is not in the store.
var ErrNotFound = errors.New("order not found")

// Store keeps the orders falatracker already handled. It is the source of
// the "was this order already uploaded?" rule: an order in the store, in any
// status, is never downloaded again.
type Store interface {
	IsProcessed(orderNumber string) bool
	Save(r Record) error
	// List returns the records with the given status, or all of them when
	// status is empty, sorted by order number.
	List(status Status) []Record
	// MarkLoaded sets the order as loaded into Dynamics and returns it.
	MarkLoaded(orderNumber string, at time.Time) (Record, error)
}

// MemoryStore is an in-memory Store. Its content is lost when the process
// exits; use FileStore in production.
type MemoryStore struct {
	mu      sync.Mutex
	records map[string]Record
}

var _ Store = (*MemoryStore)(nil)

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{records: make(map[string]Record)}
}

// IsProcessed implements [Store].
func (s *MemoryStore) IsProcessed(orderNumber string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.records[orderNumber]
	return ok
}

// Save implements [Store]. It replaces any previous record of the order.
func (s *MemoryStore) Save(r Record) error {
	if r.OrderNumber == "" {
		return errors.New("save record: empty order number")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[r.OrderNumber] = r
	return nil
}

// List implements [Store].
func (s *MemoryStore) List(status Status) []Record {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Record, 0, len(s.records))
	for _, r := range s.records {
		if status == "" || r.Status == status {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OrderNumber < out[j].OrderNumber })
	return out
}

// MarkLoaded implements [Store].
func (s *MemoryStore) MarkLoaded(orderNumber string, at time.Time) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.records[orderNumber]
	if !ok {
		return Record{}, fmt.Errorf("mark %s as loaded: %w", orderNumber, ErrNotFound)
	}
	r.Status = StatusLoaded
	r.LoadedAt = &at
	s.records[orderNumber] = r
	return r, nil
}

// snapshot returns a copy of all records (used by FileStore).
func (s *MemoryStore) snapshot() map[string]Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make(map[string]Record, len(s.records))
	for k, v := range s.records {
		cp[k] = v
	}
	return cp
}

// restore replaces all records (used by FileStore to roll back a failed write).
func (s *MemoryStore) restore(records map[string]Record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = records
}
