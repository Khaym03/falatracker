package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
)

type server struct {
	httpServer *http.Server
	cancel     context.CancelFunc
}

func newServer(cancel context.CancelFunc) *server {
	mux := http.NewServeMux()

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", 2027),
		Handler: mux,
	}

	s := &server{
		httpServer: srv,
		cancel:     cancel,
	}

	mux.HandleFunc("GET /", s.healthcheck)
	mux.HandleFunc("POST /shutdown", s.handlerShutdown)

	return s
}

func (s *server) start() error {
	log.Printf("falatracker is running on http://localhost%s", s.httpServer.Addr)
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	if err := s.httpServer.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *server) healthcheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *server) shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *server) handlerShutdown(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	go s.cancel()
}
