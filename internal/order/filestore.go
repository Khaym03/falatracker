package order

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileStore is a Store persisted as a JSON file. It is meant for the low
// volume of this process (a few orders per month): the whole file is
// rewritten on every change.
//
// Writes are atomic: the data goes to a temporary file that then replaces the
// real one, so a crash never leaves a half-written store. If the write fails
// the change is rolled back in memory too, so memory and disk never diverge.
type FileStore struct {
	path string
	mem  *MemoryStore
	mu   sync.Mutex // serializes change+write
}

var _ Store = (*FileStore)(nil)

// fileFormat is the layout of the JSON file.
type fileFormat struct {
	Orders []Record `json:"orders"`
}

// OpenFileStore loads the store at path, creating its directory if needed.
// A missing file is an empty store.
func OpenFileStore(path string) (*FileStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create store directory: %w", err)
	}

	s := &FileStore{path: path, mem: NewMemoryStore()}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read store: %w", err)
	}

	var f fileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse store %s: %w", path, err)
	}
	for _, r := range f.Orders {
		if err := s.mem.Save(r); err != nil {
			return nil, fmt.Errorf("load store %s: %w", path, err)
		}
	}
	return s, nil
}

// IsProcessed implements [Store].
func (s *FileStore) IsProcessed(orderNumber string) bool {
	return s.mem.IsProcessed(orderNumber)
}

// List implements [Store].
func (s *FileStore) List(status Status) []Record {
	return s.mem.List(status)
}

// Save implements [Store].
func (s *FileStore) Save(r Record) error {
	return s.change(func() error { return s.mem.Save(r) })
}

// MarkLoaded implements [Store].
func (s *FileStore) MarkLoaded(orderNumber string, at time.Time) (Record, error) {
	var out Record
	err := s.change(func() error {
		r, err := s.mem.MarkLoaded(orderNumber, at)
		out = r
		return err
	})
	return out, err
}

// change applies fn in memory and persists it, rolling back on failure.
func (s *FileStore) change(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	before := s.mem.snapshot()
	if err := fn(); err != nil {
		return err
	}
	if err := s.write(); err != nil {
		s.mem.restore(before)
		return err
	}
	return nil
}

// write saves all records to a temporary file and renames it over the store.
func (s *FileStore) write() error {
	data, err := json.MarshalIndent(fileFormat{Orders: s.mem.List("")}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode store: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("write store: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write store: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("write store: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write store: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("write store: %w", err)
	}
	return nil
}
