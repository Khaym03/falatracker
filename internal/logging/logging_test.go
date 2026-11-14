package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevelopmentUsesTextHandler(t *testing.T) {
	var buf bytes.Buffer
	log, closeFn, err := New(Options{Env: EnvDevelopment, Level: slog.LevelInfo, Console: &buf})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer closeFn()

	log.Info("hello", "order", "123")

	out := buf.String()
	if !strings.Contains(out, "msg=hello") || !strings.Contains(out, "order=123") {
		t.Errorf("unexpected text output: %q", out)
	}
}

func TestProductionUsesJSONHandlerAndWritesFile(t *testing.T) {
	var buf bytes.Buffer
	path := filepath.Join(t.TempDir(), "nested", "app.log")

	log, closeFn, err := New(Options{Env: EnvProduction, Level: slog.LevelInfo, Console: &buf, FilePath: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	log.Info("hello", "order", "123")
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}

	fileData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !bytes.Equal(fileData, buf.Bytes()) {
		t.Errorf("file and console differ:\nfile:    %q\nconsole: %q", fileData, buf.Bytes())
	}

	var rec map[string]any
	if err := json.Unmarshal(fileData, &rec); err != nil {
		t.Fatalf("log line is not JSON: %v (%q)", err, fileData)
	}
	if rec["msg"] != "hello" || rec["order"] != "123" || rec["level"] != "INFO" {
		t.Errorf("unexpected record: %v", rec)
	}
}

func TestFileIsAppended(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	for i := range 2 {
		log, closeFn, err := New(Options{Env: EnvProduction, FilePath: path, Console: &bytes.Buffer{}})
		if err != nil {
			t.Fatalf("New #%d: %v", i, err)
		}
		log.Info("line")
		closeFn()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), "\n"); n != 2 {
		t.Errorf("file has %d lines, want 2", n)
	}
}

func TestLevelFiltersRecords(t *testing.T) {
	var buf bytes.Buffer
	log, _, err := New(Options{Env: EnvDevelopment, Level: slog.LevelWarn, Console: &buf})
	if err != nil {
		t.Fatal(err)
	}
	log.Info("hidden")
	log.Warn("shown")

	if strings.Contains(buf.String(), "hidden") || !strings.Contains(buf.String(), "shown") {
		t.Errorf("unexpected output: %q", buf.String())
	}
}

func TestUnknownEnv(t *testing.T) {
	if _, _, err := New(Options{Env: "staging"}); err == nil {
		t.Fatal("expected error for unknown env")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in, env string
		want    slog.Level
		wantErr bool
	}{
		{in: "", env: EnvDevelopment, want: slog.LevelDebug},
		{in: "", env: EnvProduction, want: slog.LevelInfo},
		{in: "warn", env: EnvDevelopment, want: slog.LevelWarn},
		{in: "ERROR", env: EnvProduction, want: slog.LevelError},
		{in: "verbose", env: EnvProduction, wantErr: true},
	}
	for _, tt := range tests {
		got, err := ParseLevel(tt.in, tt.env)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseLevel(%q, %q) error = %v, wantErr %v", tt.in, tt.env, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseLevel(%q, %q) = %v, want %v", tt.in, tt.env, got, tt.want)
		}
	}
}
