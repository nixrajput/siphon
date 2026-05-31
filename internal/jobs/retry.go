package jobs

import (
	"context"
	"math/rand"
	"time"
)

// Retry runs op with exponential backoff + jitter, up to maxAttempts.
// Returns the last error if all attempts fail. Respects ctx cancellation
// between attempts.
func Retry(ctx context.Context, maxAttempts int, op func() error) error {
	var err error
	delay := 100 * time.Millisecond
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = op()
		if err == nil {
			return nil
		}
		if attempt == maxAttempts {
			return err
		}
		jitter := time.Duration(rand.Int63n(int64(delay) / 2))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay + jitter):
		}
		delay *= 2
		if delay > 4*time.Second {
			delay = 4 * time.Second
		}
	}
	return err
}
