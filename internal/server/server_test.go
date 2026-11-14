package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Khaym03/falatracker/internal/order"
)

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func newTestServer(t *testing.T, stop func()) (*Server, *order.MemoryStore) {
	t.Helper()
	store := order.NewMemoryStore()
	for _, r := range []order.Record{
		{OrderNumber: "1", Status: order.StatusReady, BillingDate: "2026-11-03"},
		{OrderNumber: "2", Status: order.StatusLoaded, BillingDate: "2026-11-01"},
		{OrderNumber: "3", Status: order.StatusReady, BillingDate: "2026-12-01"},
	} {
		if err := store.Save(r); err != nil {
			t.Fatal(err)
		}
	}
	if stop == nil {
		stop = func() {}
	}
	return New(":0", store, stop, discard()), store
}

func do(t *testing.T, s *Server, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestHealth(t *testing.T) {
	s, _ := newTestServer(t, nil)
	rec := do(t, s, http.MethodGet, "/")

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["status"] != "ok" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestListOrders(t *testing.T) {
	s, _ := newTestServer(t, nil)

	tests := []struct {
		target string
		want   []string
	}{
		{"/orders", []string{"1", "2", "3"}},
		{"/orders?status=ready", []string{"1", "3"}},
		{"/orders?status=loaded", []string{"2"}},
	}
	for _, tt := range tests {
		rec := do(t, s, http.MethodGet, tt.target)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", tt.target, rec.Code)
		}
		var got []order.Record
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("%s: %v", tt.target, err)
		}
		if fmt.Sprint(numbers(got)) != fmt.Sprint(tt.want) {
			t.Errorf("%s = %v, want %v", tt.target, numbers(got), tt.want)
		}
	}
}

func TestListOrdersEmptyIsAnArray(t *testing.T) {
	s := New(":0", order.NewMemoryStore(), func() {}, discard())
	rec := do(t, s, http.MethodGet, "/orders")
	if body := rec.Body.String(); body != "[]\n" {
		t.Errorf("body = %q, want an empty JSON array", body)
	}
}

func TestListOrdersInvalidStatus(t *testing.T) {
	s, _ := newTestServer(t, nil)
	if rec := do(t, s, http.MethodGet, "/orders?status=done"); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestOrderLoaded(t *testing.T) {
	s, store := newTestServer(t, nil)

	rec := do(t, s, http.MethodPost, "/orders/1/loaded")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var got order.Record
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != order.StatusLoaded || got.LoadedAt == nil {
		t.Errorf("record = %+v", got)
	}
	if len(store.List(order.StatusReady)) != 1 {
		t.Error("order 1 should no longer be ready")
	}

	if rec := do(t, s, http.MethodPost, "/orders/404/loaded"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown order: status = %d, want 404", rec.Code)
	}
	do(t, s, http.MethodGet, "/orders/3/loaded")
	if got := numbers(store.List(order.StatusReady)); fmt.Sprint(got) != "[3]" {
		t.Errorf("GET must not mark orders as loaded; ready = %v", got)
	}
}

func TestShutdownEndpoint(t *testing.T) {
	stopped := false
	s, _ := newTestServer(t, func() { stopped = true })

	if rec := do(t, s, http.MethodGet, "/shutdown"); stopped {
		t.Errorf("GET /shutdown must not stop the app (status %d)", rec.Code)
	}
	if rec := do(t, s, http.MethodPost, "/shutdown"); rec.Code != http.StatusOK || !stopped {
		t.Errorf("POST /shutdown: status = %d, stopped = %v", rec.Code, stopped)
	}
}

func TestStartAndShutdown(t *testing.T) {
	addr := freeAddr(t)
	s := New(addr, order.NewMemoryStore(), func() {}, discard())
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	resp, err := http.Get(fmt.Sprintf("http://%s/", addr))
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestStartFailsWhenAddressInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	s := New(ln.Addr().String(), order.NewMemoryStore(), func() {}, discard())
	if err := s.Start(); err == nil {
		t.Fatal("expected an error for an address already in use")
	}
}

func numbers(rs []order.Record) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.OrderNumber
	}
	return out
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}
