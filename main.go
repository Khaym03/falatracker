package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	status := run(ctx, cancel)
	cancel()
	os.Exit(status)
}

func run(ctx context.Context, cancel context.CancelFunc) int {
	store := make(fakeStore)

	s := newServer(cancel)
	var serverErr error
	go func() {
		serverErr = s.start()
	}()

	cred, err := GetCredentialFromENV()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get cred env: %v\n", err)
	}

	browser, err := NewBrowser()
	if err != nil {
		log.Fatal(err)
	}

	t := NewTracker(browser, store, cred)
	go func() {
		t.Start(ctx, 3*time.Minute)
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to shutdown server: %v\n", err)
		return 1
	}
	if err := t.shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to shutdown tracker: %v\n", err)
		return 1
	}
	if serverErr != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", serverErr)
		return 1
	}

	return 0
}
