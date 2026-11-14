// Package server exposes the HTTP control channel used by PAD (Power
// Automate Desktop) and operators:
//
//	GET  /                          health check: {"status":"ok"}
//	GET  /orders[?status=ready]     processed orders (order.Record JSON list)
//	POST /orders/{number}/loaded    PAD confirms it loaded the order into Dynamics
//	POST /shutdown                  stops the process gracefully
//
// Typical PAD flow: check the health endpoint (start falatracker if it is
// down), GET /orders?status=ready, load each order whose billing_date has
// arrived, then POST /orders/{number}/loaded.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/Khaym03/falatracker/internal/order"
)

// OrderStore is the part of order.Store the server needs.
type OrderStore interface {
	List(status order.Status) []order.Record
	MarkLoaded(orderNumber string, at time.Time) (order.Record, error)
}

// Server is the HTTP control server.
type Server struct {
	http  *http.Server
	store OrderStore
	stop  func()
	log   *slog.Logger
}

// New returns a Server listening on addr. stop is called when a shutdown is
// requested over HTTP or when the server fails unexpectedly.
func New(addr string, store OrderStore, stop func(), log *slog.Logger) *Server {
	s := &Server{store: store, stop: stop, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleHealth)
	mux.HandleFunc("GET /orders", s.handleListOrders)
	mux.HandleFunc("POST /orders/{number}/loaded", s.handleOrderLoaded)
	mux.HandleFunc("POST /shutdown", s.handleShutdown)

	s.http = &http.Server{Addr: addr, Handler: mux}
	return s
}

// Handler returns the server's HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.http.Handler
}

// Start binds the address and serves requests in the background. A failure
// to bind is returned right away.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.http.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.http.Addr, err)
	}
	s.log.Info("http server listening", "addr", ln.Addr().String())

	go func() {
		if err := s.http.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Error("http server failed", "error", err)
			s.stop()
		}
	}()
	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.http.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListOrders(w http.ResponseWriter, r *http.Request) {
	status := order.Status(r.URL.Query().Get("status"))
	if status != "" && !status.Valid() {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("invalid status %q: use %q or %q", status, order.StatusReady, order.StatusLoaded),
		})
		return
	}
	writeJSON(w, http.StatusOK, s.store.List(status))
}

func (s *Server) handleOrderLoaded(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	rec, err := s.store.MarkLoaded(number, time.Now())
	switch {
	case errors.Is(err, order.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case err != nil:
		s.log.Error("mark order loaded", "order", number, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	default:
		s.log.Info("order marked as loaded", "order", number, "remote_addr", r.RemoteAddr)
		writeJSON(w, http.StatusOK, rec)
	}
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	s.log.Info("shutdown requested over http", "remote_addr", r.RemoteAddr)
	w.WriteHeader(http.StatusOK)
	s.stop()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
