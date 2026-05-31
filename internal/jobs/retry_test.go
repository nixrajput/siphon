package jobs

import (
	"context"
	"errors"
	"testing"
)

func TestRetry_SucceedsFirstAttempt(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), 3, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d; want 1", calls)
	}
}

func TestRetry_SucceedsLaterAttempt(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), 3, func() error {
		calls++
		if calls < 2 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d; want 2", calls)
	}
}

func TestRetry_AllAttemptsFail(t *testing.T) {
	calls := 0
	last := errors.New("boom")
	err := Retry(context.Background(), 3, func() error {
		calls++
		return last
	})
	if !errors.Is(err, last) {
		t.Fatalf("err = %v; want %v", err, last)
	}
	if calls != 3 {
		t.Fatalf("calls = %d; want 3", calls)
	}
}

func TestRetry_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	err := Retry(ctx, 3, func() error {
		calls++
		return errors.New("transient")
	})
	// First op runs, fails; the backoff select then sees ctx.Done() and
	// returns ctx.Err() promptly without hanging.
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v; want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d; want 1", calls)
	}
}
