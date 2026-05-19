package resilience

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedState(t *testing.T) {
	cb := NewCircuitBreaker(DefaultConfig("test"))
	if cb.State() != StateClosed {
		t.Error("initial state should be CLOSED")
	}

	err := cb.Execute(context.Background(), func() error { return nil })
	if err != nil {
		t.Errorf("Execute in closed state returned: %v", err)
	}
}

func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
	cfg := DefaultConfig("test")
	cfg.FailureThreshold = 3
	cb := NewCircuitBreaker(cfg)

	failFn := func() error { return errors.New("boom") }
	for i := 0; i < cfg.FailureThreshold; i++ {
		_ = cb.Execute(context.Background(), failFn)
	}

	if cb.State() != StateOpen {
		t.Error("should be OPEN after reaching failure threshold")
	}
}

func TestCircuitBreaker_RejectsWhenOpen(t *testing.T) {
	cfg := DefaultConfig("test")
	cfg.FailureThreshold = 1
	cfg.Timeout = 10 * time.Second
	cb := NewCircuitBreaker(cfg)

	_ = cb.Execute(context.Background(), func() error { return errors.New("fail") })

	err := cb.Execute(context.Background(), func() error { return nil })
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got: %v", err)
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cfg := DefaultConfig("test")
	cfg.FailureThreshold = 1
	cfg.Timeout = 50 * time.Millisecond
	cfg.SuccessThreshold = 1
	cb := NewCircuitBreaker(cfg)

	// Trip the breaker.
	_ = cb.Execute(context.Background(), func() error { return errors.New("fail") })

	// Wait for timeout.
	time.Sleep(80 * time.Millisecond)

	// Should transition to half-open on next call.
	err := cb.Execute(context.Background(), func() error { return nil })
	if err != nil {
		t.Errorf("half-open should allow request, got: %v", err)
	}

	// With success threshold 1, should now be closed.
	if cb.State() != StateClosed {
		t.Error("should be CLOSED after success in half-open")
	}
}

func TestCircuitBreaker_HalfOpenReopensOnFailure(t *testing.T) {
	cfg := DefaultConfig("test")
	cfg.FailureThreshold = 1
	cfg.Timeout = 50 * time.Millisecond
	cb := NewCircuitBreaker(cfg)

	// Trip it.
	_ = cb.Execute(context.Background(), func() error { return errors.New("fail") })

	// Wait for timeout.
	time.Sleep(80 * time.Millisecond)

	// Fail in half-open.
	_ = cb.Execute(context.Background(), func() error { return errors.New("fail again") })

	// Should be open again.
	if cb.State() != StateOpen {
		t.Error("should be OPEN after failure in half-open")
	}
}

func TestCircuitBreaker_ResetsOnSuccess(t *testing.T) {
	cfg := DefaultConfig("test")
	cfg.FailureThreshold = 3
	cb := NewCircuitBreaker(cfg)

	// 2 failures (below threshold).
	failFn := func() error { return errors.New("fail") }
	_ = cb.Execute(context.Background(), failFn)
	_ = cb.Execute(context.Background(), failFn)

	// 1 success resets failure count.
	_ = cb.Execute(context.Background(), func() error { return nil })

	// 2 more failures — should NOT open because count was reset.
	_ = cb.Execute(context.Background(), failFn)
	_ = cb.Execute(context.Background(), failFn)

	if cb.State() != StateClosed {
		t.Error("should still be CLOSED because failures were reset by success")
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cfg := DefaultConfig("test")
	cfg.FailureThreshold = 100
	cb := NewCircuitBreaker(cfg)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cb.Execute(context.Background(), func() error { return nil })
		}()
	}
	wg.Wait()

	if cb.State() != StateClosed {
		t.Error("should remain CLOSED after concurrent successes")
	}
}

func TestCircuitBreaker_Stats(t *testing.T) {
	cfg := DefaultConfig("test")
	cfg.FailureThreshold = 5
	cb := NewCircuitBreaker(cfg)

	_ = cb.Execute(context.Background(), func() error { return errors.New("fail") })
	_ = cb.Execute(context.Background(), func() error { return nil })

	stats := cb.Stats()
	if stats.State != StateClosed {
		t.Errorf("State = %v, want CLOSED", stats.State)
	}
	if stats.Failures != 0 {
		t.Errorf("Failures = %d, want 0 (reset by success)", stats.Failures)
	}
}

func TestCircuitBreaker_HalfOpenMaxInFlight(t *testing.T) {
	cfg := DefaultConfig("test")
	cfg.FailureThreshold = 1
	cfg.Timeout = 50 * time.Millisecond
	cfg.SuccessThreshold = 5
	cfg.HalfOpenMax = 1
	cb := NewCircuitBreaker(cfg)

	// Trip it.
	_ = cb.Execute(context.Background(), func() error { return errors.New("fail") })
	time.Sleep(80 * time.Millisecond)

	// Use a sync mechanism to ensure first call is inside fn before second call.
	inside := make(chan struct{})
	continueFn := make(chan struct{})
	var firstErr error
	done := make(chan struct{})
	go func() {
		firstErr = cb.Execute(context.Background(), func() error {
			close(inside)
			<-continueFn
			return nil
		})
		close(done)
	}()

	// Wait until the first call's fn is executing (past before()).
	<-inside

	// Second request should be rejected because halfOpenInFlight == HalfOpenMax.
	err := cb.Execute(context.Background(), func() error { return nil })
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("second half-open request should be rejected, got: %v", err)
	}

	// Let the first call complete.
	close(continueFn)
	<-done
	if firstErr != nil {
		t.Errorf("first half-open request should succeed, got: %v", firstErr)
	}
}

func TestCircuitBreaker_ContextCancellation(t *testing.T) {
	cb := NewCircuitBreaker(DefaultConfig("test"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Context cancellation doesn't affect the breaker — fn still runs.
	// The breaker only checks its own state in before().
	err := cb.Execute(ctx, func() error { return nil })
	if err != nil {
		t.Errorf("breaker should pass through, got: %v", err)
	}
}
