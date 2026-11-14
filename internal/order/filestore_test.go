package order

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileStore(t *testing.T) {
	storeContract(t, func(t *testing.T) Store {
		s, err := OpenFileStore(filepath.Join(t.TempDir(), "orders.json"))
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}

// The whole point of FileStore: data survives a restart.
func TestFileStorePersistsAcrossRestarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "orders.json") // directory created on open

	s, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("OpenFileStore: %v", err)
	}
	rec := record("1000001", StatusReady)
	rec.Files = Files{Products: "p.xlsx", Distribution: "d.xlsx", Standard: "s.xlsx"}
	rec.Issues = []string{"issue"}
	if err := s.Save(rec); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MarkLoaded("1000001", time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if !reopened.IsProcessed("1000001") {
		t.Fatal("order lost after restart")
	}
	got := reopened.List("")[0]
	if got.Status != StatusLoaded || got.LoadedAt == nil || got.Files != rec.Files || got.Issues[0] != "issue" {
		t.Errorf("reloaded record = %+v", got)
	}
}

func TestFileStoreFileFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orders.json")
	s, _ := OpenFileStore(path)
	if err := s.Save(record("1", StatusReady)); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Orders []map[string]any `json:"orders"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("store is not valid JSON: %v", err)
	}
	if len(f.Orders) != 1 || f.Orders[0]["order_number"] != "1" || f.Orders[0]["status"] != "ready" {
		t.Errorf("unexpected content: %s", data)
	}

	// No temporary files left behind.
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("directory has %d entries, want only orders.json", len(entries))
	}
}

func TestOpenFileStoreCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orders.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenFileStore(path); err == nil || !strings.Contains(err.Error(), "parse store") {
		t.Fatalf("error = %v, want parse error", err)
	}
}

// If the file cannot be written, memory must not keep the change either.
func TestFileStoreRollsBackOnWriteFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "store")
	s, err := OpenFileStore(filepath.Join(dir, "orders.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(record("1", StatusReady)); err != nil {
		t.Fatal(err)
	}

	// Replace the directory with a file so any write fails.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.Save(record("2", StatusReady)); err == nil {
		t.Fatal("expected a write error")
	}
	if s.IsProcessed("2") {
		t.Error("failed save must be rolled back")
	}
	if _, err := s.MarkLoaded("1", time.Now()); err == nil {
		t.Fatal("expected a write error")
	}
	if got := s.List("")[0].Status; got != StatusReady {
		t.Errorf("failed MarkLoaded must be rolled back, status = %s", got)
	}
}
