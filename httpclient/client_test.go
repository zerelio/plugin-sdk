package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNew_DefaultConfig(t *testing.T) {
	c := New(DefaultConfig("test"))
	if c == nil {
		t.Fatal("New returned nil")
	}
	if c.http.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", c.http.Timeout)
	}
}

func TestClient_Get_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Method = %s, want GET", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	c := New(DefaultConfig("test"))
	resp, err := c.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Status = %d, want 200", resp.StatusCode)
	}
}

func TestClient_Post_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %s, want application/json", ct)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	c := New(DefaultConfig("test"))
	resp, err := c.Post(context.Background(), server.URL, "application/json", []byte(`{"key":"value"}`))
	if err != nil {
		t.Fatalf("Post() error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Status = %d, want 201", resp.StatusCode)
	}
}

func TestClient_RetryOnServerError(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultConfig("test")
	cfg.Retry.BaseDelay = 1 * time.Millisecond
	c := New(cfg)

	resp, err := c.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	defer resp.Body.Close()
	if calls != 3 {
		t.Errorf("calls = %d, want 3 (initial + 2 retries)", calls)
	}
}

func TestClient_NoRetryOnClientError(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	cfg := DefaultConfig("test")
	c := New(cfg)

	resp, err := c.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	defer resp.Body.Close()
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on 4xx)", calls)
	}
}

func TestClient_CircuitBreakerOpens(t *testing.T) {
	cfg := DefaultConfig("test")
	cfg.Retry.Enabled = false
	cfg.CircuitBreaker.FailureThreshold = 2
	cfg.CircuitBreaker.Timeout = 5 * time.Second

	c := New(cfg)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// Trip the breaker.
	for i := 0; i < 2; i++ {
		resp, _ := c.Get(context.Background(), server.URL)
		if resp != nil {
			resp.Body.Close()
		}
	}

	// Next call should fail with circuit open.
	_, err := c.Get(context.Background(), server.URL)
	if err == nil {
		t.Error("expected error after circuit opens")
	}
}

func TestClient_RateLimiting(t *testing.T) {
	cfg := DefaultConfig("test")
	cfg.RateLimit = 100 // High enough to not block.
	cfg.RateBurst = 5
	cfg.Retry.Enabled = false

	c := New(cfg)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Should succeed within burst.
	for i := 0; i < 5; i++ {
		resp, err := c.Get(context.Background(), server.URL)
		if err != nil {
			t.Errorf("call %d error: %v", i, err)
		}
		if resp != nil {
			resp.Body.Close()
		}
	}
}

func TestClient_DisabledResilience(t *testing.T) {
	cfg := ClientConfig{
		Name:            "test-minimal",
		ConnectTimeout:  5 * time.Second,
		RequestTimeout:  10 * time.Second,
		MaxIdleConns:    50,
		UserAgent:       "Test/1.0",
		Retry:           RetryPolicy{Enabled: false},
		CircuitBreaker:  CircuitBreakerPolicy{Enabled: false},
		RateLimit:       0, // No rate limiting.
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(cfg)
	resp, err := c.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	defer resp.Body.Close()

	if c.cb != nil {
		t.Error("circuit breaker should be nil when disabled")
	}
	if c.rl != nil {
		t.Error("rate limiter should be nil when RateLimit is 0")
	}
}

func TestRequest_SetHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get("X-Custom"); v != "test-value" {
			t.Errorf("X-Custom = %q, want %q", v, "test-value")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(DefaultConfig("test"))
	req := NewRequest(context.Background(), http.MethodGet, server.URL, nil)
	req.SetHeader("X-Custom", "test-value")

	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do() error: %v", err)
	}
	defer resp.Body.Close()
}

func TestCircuitBreakerStats_Disabled(t *testing.T) {
	cfg := DefaultConfig("test")
	cfg.CircuitBreaker.Enabled = false
	c := New(cfg)
	if stats := c.CircuitBreakerStats(); stats != nil {
		t.Error("CircuitBreakerStats should be nil when disabled")
	}
}
