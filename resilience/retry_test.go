package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetry_SucceedsImmediately(t *testing.T) {
	cfg := DefaultRetryConfig()
	err := Retry(context.Background(), cfg, alwaysRetry, func() error { return nil })
	if err != nil {
		t.Errorf("expected nil, got: %v", err)
	}
}

func TestRetry_SucceedsAfterRetries(t *testing.T) {
	cfg := DefaultRetryConfig()
	cfg.BaseDelay = 1 * time.Millisecond

	callCount := 0
	err := Retry(context.Background(), cfg, alwaysRetry, func() error {
		callCount++
		if callCount < 3 {
			return errors.New("fail")
		}
		return nil
	})
	if err != nil {
		t.Errorf("expected nil, got: %v", err)
	}
	if callCount != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestRetry_ExhaustsAttempts(t *testing.T) {
	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = 2
	cfg.BaseDelay = 1 * time.Millisecond

	callCount := 0
	err := Retry(context.Background(), cfg, alwaysRetry, func() error {
		callCount++
		return errors.New("persistent fail")
	})
	if err == nil {
		t.Error("expected error after exhausting attempts")
	}
	if callCount != 2 {
		t.Errorf("callCount = %d, want 2", callCount)
	}
}

func TestRetry_StopsOnNonRetryable(t *testing.T) {
	cfg := DefaultRetryConfig()
	cfg.BaseDelay = 1 * time.Millisecond

	neverRetry := func(err error) bool { return false }
	callCount := 0
	err := Retry(context.Background(), cfg, neverRetry, func() error {
		callCount++
		return errors.New("non-retryable")
	})
	if err == nil {
		t.Error("expected error")
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1 (no retry)", callCount)
	}
}

func TestRetry_ContextCancellation(t *testing.T) {
	cfg := DefaultRetryConfig()
	cfg.BaseDelay = 1 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_ = Retry(ctx, cfg, alwaysRetry, func() error {
		callCount++
		return errors.New("fail")
	})
	// Should have stopped after context cancellation.
	// We can't assert exact count, just that it didn't run forever.
}

func TestBackoff_NoJitter(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 100 * time.Millisecond},
		{1, 200 * time.Millisecond},
		{2, 400 * time.Millisecond},
		{3, 800 * time.Millisecond},
	}
	for _, tc := range tests {
		got := backoff(100*time.Millisecond, 5*time.Second, tc.attempt, false)
		if got != tc.want {
			t.Errorf("backoff(attempt=%d) = %v, want %v", tc.attempt, got, tc.want)
		}
	}
}

func TestBackoff_Capped(t *testing.T) {
	got := backoff(100*time.Millisecond, 200*time.Millisecond, 5, false)
	if got != 200*time.Millisecond {
		t.Errorf("backoff should be capped at max, got %v", got)
	}
}

func alwaysRetry(error) bool { return true }
