package resilience

import (
	"context"
	"math/rand/v2"
	"time"
)

// RetryConfig holds configuration for retry with exponential backoff.
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (including the first).
	MaxAttempts int
	// BaseDelay is the initial backoff duration.
	BaseDelay time.Duration
	// MaxDelay caps the backoff duration.
	MaxDelay time.Duration
	// Jitter adds randomness to avoid thundering herd.
	Jitter bool
}

// DefaultRetryConfig returns sensible retry defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    5 * time.Second,
		Jitter:      true,
	}
}

// IsRetryable determines if an error warrants a retry.
type IsRetryable func(error) bool

// Retry executes fn with exponential backoff. It stops when fn succeeds,
// MaxAttempts is reached, or the context is cancelled.
func Retry(ctx context.Context, cfg RetryConfig, retryable IsRetryable, fn func() error) error {
	var err error
	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if !retryable(err) || attempt == cfg.MaxAttempts-1 {
			return err
		}

		delay := backoff(cfg.BaseDelay, cfg.MaxDelay, attempt, cfg.Jitter)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}

func backoff(base, max time.Duration, attempt int, jitter bool) time.Duration {
	d := base
	for i := 0; i < attempt; i++ {
		d *= 2
		if d > max {
			d = max
			break
		}
	}
	if jitter {
		d = time.Duration(float64(d) * (0.5 + rand.Float64()*0.5))
	}
	return d
}
