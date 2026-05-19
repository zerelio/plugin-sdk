// Package httpclient provides a factory for pre-configured HTTP clients
// with timeouts, retries, circuit breaker protection, and rate limiting.
package httpclient

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/knowledgetree-dev/knowledge-tree-sdk/resilience"
	"golang.org/x/time/rate"
)

// ClientConfig holds configuration for an HTTP client.
type ClientConfig struct {
	// Name identifies this client in logs and metrics.
	Name string
	// ConnectTimeout is the TCP connection timeout.
	ConnectTimeout time.Duration
	// RequestTimeout is the overall request timeout.
	RequestTimeout time.Duration
	// MaxIdleConns controls the global idle connection pool size.
	MaxIdleConns int
	// MaxIdleConnsPerHost controls per-host idle connections.
	MaxIdleConnsPerHost int
	// UserAgent is sent with every request.
	UserAgent string
	// Retry configures retry behavior. Zero value disables retries.
	Retry RetryPolicy
	// CircuitBreaker configures circuit breaker behavior. Zero value disables.
	CircuitBreaker CircuitBreakerPolicy
	// RateLimit controls requests per second. Zero means unlimited.
	RateLimit float64
	// RateBurst is the token bucket burst size.
	RateBurst int
}

// RetryPolicy mirrors resilience.RetryConfig for user-friendly defaults.
type RetryPolicy struct {
	// Enabled enables retry behavior.
	Enabled bool
	// MaxAttempts is the maximum number of attempts.
	MaxAttempts int
	// BaseDelay is the initial backoff duration.
	BaseDelay time.Duration
	// MaxDelay caps the backoff.
	MaxDelay time.Duration
}

// CircuitBreakerPolicy configures circuit breaker behavior.
type CircuitBreakerPolicy struct {
	// Enabled enables circuit breaker protection.
	Enabled bool
	// FailureThreshold is consecutive failures before opening.
	FailureThreshold int
	// SuccessThreshold is consecutive successes to close from half-open.
	SuccessThreshold int
	// Timeout is the open-state duration.
	Timeout time.Duration
}

// DefaultConfig returns a production-ready default configuration.
func DefaultConfig(name string) ClientConfig {
	return ClientConfig{
		Name:                name,
		ConnectTimeout:      5 * time.Second,
		RequestTimeout:      30 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		UserAgent:           "KnowledgeTree/1.0",
		Retry: RetryPolicy{
			Enabled:     true,
			MaxAttempts: 3,
			BaseDelay:   200 * time.Millisecond,
			MaxDelay:    10 * time.Second,
		},
		CircuitBreaker: CircuitBreakerPolicy{
			Enabled:          true,
			FailureThreshold: 5,
			SuccessThreshold: 3,
			Timeout:          30 * time.Second,
		},
		RateLimit: 50,
		RateBurst: 10,
	}
}

// Client wraps an http.Client with resilience patterns.
type Client struct {
	http *http.Client
	cfg  ClientConfig
	cb   *resilience.CircuitBreaker
	rl   *rate.Limiter
}

// New creates a new resilient HTTP client.
func New(cfg ClientConfig) *Client {
	dialer := &net.Dialer{
		Timeout:   cfg.ConnectTimeout,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		DialContext:         dialer.DialContext,
		MaxIdleConns:        cfg.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   cfg.RequestTimeout,
	}

	c := &Client{
		http: httpClient,
		cfg:  cfg,
	}

	if cfg.CircuitBreaker.Enabled {
		c.cb = resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
			Name:             cfg.Name,
			FailureThreshold: cfg.CircuitBreaker.FailureThreshold,
			SuccessThreshold: cfg.CircuitBreaker.SuccessThreshold,
			Timeout:          cfg.CircuitBreaker.Timeout,
			HalfOpenMax:      1,
		})
	}

	if cfg.RateLimit > 0 {
		c.rl = rate.NewLimiter(rate.Limit(cfg.RateLimit), cfg.RateBurst)
	}

	return c
}

// Do executes an HTTP request with retry, circuit breaker, and rate limiting.
func (c *Client) Do(req *Request) (*http.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Rate limit.
	if c.rl != nil {
		if err := c.rl.Wait(ctx); err != nil {
			return nil, err
		}
	}

	// Build the retry config.
	retryCfg := resilience.RetryConfig{
		MaxAttempts: 1,
	}
	if c.cfg.Retry.Enabled {
		retryCfg = resilience.RetryConfig{
			MaxAttempts: c.cfg.Retry.MaxAttempts,
			BaseDelay:   c.cfg.Retry.BaseDelay,
			MaxDelay:    c.cfg.Retry.MaxDelay,
			Jitter:      true,
		}
	}

	var resp *http.Response
	var lastErr error

	fn := func() error {
		httpReq := req.buildHTTP()
		r, err := c.http.Do(httpReq)
		if err != nil {
			lastErr = err
			return err
		}
		// Treat 5xx and 429 as retryable.
		if r.StatusCode >= 500 || r.StatusCode == 429 {
			r.Body.Close()
			lastErr = &httpError{StatusCode: r.StatusCode}
			return lastErr
		}
		resp = r
		return nil
	}

	retryable := func(error) bool { return true }

	if c.cb != nil {
		lastErr = c.cb.Execute(ctx, func() error {
			return resilience.Retry(ctx, retryCfg, retryable, fn)
		})
	} else {
		lastErr = resilience.Retry(ctx, retryCfg, retryable, fn)
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return resp, nil
}

// Get is a convenience method for GET requests.
func (c *Client) Get(ctx context.Context, url string) (*http.Response, error) {
	return c.Do(NewRequest(ctx, http.MethodGet, url, nil))
}

// Post is a convenience method for POST requests.
func (c *Client) Post(ctx context.Context, url, contentType string, body []byte) (*http.Response, error) {
	req := NewRequest(ctx, http.MethodPost, url, body)
	req.SetHeader("Content-Type", contentType)
	return c.Do(req)
}

// CircuitBreakerStats returns circuit breaker statistics, or nil if disabled.
func (c *Client) CircuitBreakerStats() *resilience.Stats {
	if c.cb == nil {
		return nil
	}
	s := c.cb.Stats()
	return &s
}

// httpError wraps an HTTP status code as an error.
type httpError struct {
	StatusCode int
}

func (e *httpError) Error() string {
	return http.StatusText(e.StatusCode)
}
