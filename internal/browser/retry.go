package browser

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Retry calls fn up to attempts times, waiting delay between failed attempts.
// It stops early when ctx is done. The returned error wraps the last failure.
func Retry(ctx context.Context, attempts int, delay time.Duration, fn func() error) error {
	attempts = max(attempts, 1)

	for attempt := 1; ; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		if attempt >= attempts || ctx.Err() != nil {
			return fmt.Errorf("failed after %d attempt(s): %w", attempt, err)
		}
		if sleepErr := Sleep(ctx, delay); sleepErr != nil {
			return fmt.Errorf("failed after %d attempt(s): %w", attempt, errors.Join(err, sleepErr))
		}
	}
}

// Sleep waits for d, returning early with ctx.Err() if ctx is done.
func Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
