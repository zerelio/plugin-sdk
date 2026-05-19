// Package resilience provides reliability patterns for external service calls.
package resilience

import (
	"context"
	"errors"
	"sync"
	"time"
)

// State represents the circuit breaker state.
type State int

const (
	// StateClosed means requests flow through normally.
	StateClosed State = iota
	// StateOpen means requests are rejected immediately.
	StateOpen
	// StateHalfOpen means a limited number of probe requests are allowed.
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreakerConfig holds configuration for a circuit breaker.
type CircuitBreakerConfig struct {
	// Name identifies this breaker in logs and metrics.
	Name string
	// FailureThreshold is the number of consecutive failures before opening.
	FailureThreshold int
	// SuccessThreshold is the consecutive successes in half-open to close.
	SuccessThreshold int
	// Timeout is how long to wait in the open state before half-open.
	Timeout time.Duration
	// HalfOpenMax is the number of probe requests allowed in half-open.
	HalfOpenMax int
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:             name,
		FailureThreshold: 5,
		SuccessThreshold: 3,
		Timeout:          30 * time.Second,
		HalfOpenMax:      1,
	}
}

// ErrCircuitOpen is returned when the circuit breaker rejects a request.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// CircuitBreaker implements the circuit breaker pattern. It is safe for
// concurrent use.
type CircuitBreaker struct {
	cfg   CircuitBreakerConfig
	mu    sync.Mutex
	state State

	failures  int
	successes int
	lastFail  time.Time

	halfOpenInFlight int
}

// NewCircuitBreaker creates a new circuit breaker with the given config.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		cfg:   cfg,
		state: StateClosed,
	}
}

// Execute runs fn through the circuit breaker. If the breaker is open and
// the timeout hasn't elapsed, it returns ErrCircuitOpen immediately.
// If fn returns a non-nil error, the breaker records a failure; otherwise
// it records a success.
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	if err := cb.before(); err != nil {
		return err
	}

	err := fn()
	cb.after(err)
	return err
}

// State returns the current circuit breaker state.
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.currentState()
}

// Stats returns current circuit breaker statistics.
type Stats struct {
	State     State
	Failures  int
	Successes int
}

// Stats returns current statistics.
func (cb *CircuitBreaker) Stats() Stats {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return Stats{
		State:     cb.currentState(),
		Failures:  cb.failures,
		Successes: cb.successes,
	}
}

func (cb *CircuitBreaker) before() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return nil
	case StateOpen:
		if time.Since(cb.lastFail) > cb.cfg.Timeout {
			cb.state = StateHalfOpen
			cb.successes = 0
			cb.halfOpenInFlight = 0
			// Fall through to half-open handling below.
		} else {
			return ErrCircuitOpen
		}
		fallthrough
	case StateHalfOpen:
		if cb.halfOpenInFlight >= cb.cfg.HalfOpenMax {
			return ErrCircuitOpen
		}
		cb.halfOpenInFlight++
		return nil
	}
	return nil
}

func (cb *CircuitBreaker) after(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err == nil {
		cb.onSuccess()
	} else {
		cb.onFailure()
	}
}

func (cb *CircuitBreaker) onSuccess() {
	cb.failures = 0

	switch cb.state {
	case StateHalfOpen:
		cb.successes++
		if cb.successes >= cb.cfg.SuccessThreshold {
			cb.state = StateClosed
			cb.successes = 0
		}
	}
}

func (cb *CircuitBreaker) onFailure() {
	cb.lastFail = time.Now()
	cb.successes = 0

	switch cb.state {
	case StateClosed:
		cb.failures++
		if cb.failures >= cb.cfg.FailureThreshold {
			cb.state = StateOpen
		}
	case StateHalfOpen:
		cb.state = StateOpen
		cb.failures = cb.cfg.FailureThreshold
	}
}

// currentState returns the state, transitioning from open to half-open
// if the timeout has elapsed.
func (cb *CircuitBreaker) currentState() State {
	if cb.state == StateOpen && time.Since(cb.lastFail) > cb.cfg.Timeout {
		return StateHalfOpen
	}
	return cb.state
}
