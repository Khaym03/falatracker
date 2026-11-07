package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

type Tracker struct {
	browser *rod.Browser
	store   Store
	cred    credential
}

func NewTracker(b *rod.Browser, store Store, cred credential) *Tracker {
	return &Tracker{
		browser: b,
		store:   store,
		cred:    cred,
	}
}

// Start runs the tracker periodically every 'interval' until the context is cancelled
func (t *Tracker) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("Tracker started. It will run every %v", interval)

	// Optional: Run immediately the first time before waiting for the ticker
	if err := t.Run(ctx); err != nil {
		log.Printf("Error in initial execution: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping the tracker...")
			return
		case <-ticker.C:
			log.Println("Executing scheduled Login and Collect cycle...")
			if err := t.Run(ctx); err != nil {
				log.Printf("Error during cycle execution: %v", err)
				// Here you can decide whether to continue or perform an internal retry
			}
		}
	}
}

func (t *Tracker) Run(ctx context.Context) error {
	pages, err := t.browser.Pages()
	if !pages.Empty() && len(pages) > 1 {
		for _, p := range pages[1:] {
			p.MustClose()
		}
	}

	page, err := t.browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		log.Fatal(err)
	}

	err = Login(ctx, page, t.cred)
	if err != nil {
		return err
	}

	return CollectOC(ctx, page, t.store)
}

func (t *Tracker) shutdown(ctx context.Context) error {
	errChan := make(chan error, 1)

	go func() {
		errChan <- t.browser.Close()
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("Browser shutdown exceeded the time limit: %w", ctx.Err())
	case err := <-errChan:
		return err
	}
}
