package browser

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetrySucceedsAfterFailures(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), 3, time.Millisecond, func() error {
		calls++
		if calls < 3 {
			return errors.New("not yet")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestRetryReturnsLastError(t *testing.T) {
	errLast := errors.New("last")
	calls := 0
	err := Retry(context.Background(), 2, time.Millisecond, func() error {
		calls++
		if calls == 2 {
			return errLast
		}
		return errors.New("first")
	})
	if !errors.Is(err, errLast) {
		t.Fatalf("error = %v, want %v", err, errLast)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestRetryAtLeastOnce(t *testing.T) {
	calls := 0
	_ = Retry(context.Background(), 0, time.Millisecond, func() error {
		calls++
		return errors.New("fail")
	})
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestRetryStopsWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	start := time.Now()

	err := Retry(ctx, 5, time.Hour, func() error {
		calls++
		cancel()
		return errors.New("fail")
	})

	if err == nil {
		t.Fatal("expected an error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
	if time.Since(start) > time.Second {
		t.Error("Retry kept waiting after cancellation")
	}
}

func TestSleep(t *testing.T) {
	if err := Sleep(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("Sleep: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("Sleep error = %v, want context.Canceled", err)
	}
}
