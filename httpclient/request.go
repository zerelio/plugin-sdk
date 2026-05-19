package httpclient

import (
	"bytes"
	"context"
	"net/http"
)

// Request wraps an http.Request with convenience methods.
type Request struct {
	ctx    context.Context
	method string
	url    string
	body   []byte
	header http.Header
}

// NewRequest creates a new request with the given method, URL, and optional body.
func NewRequest(ctx context.Context, method, url string, body []byte) *Request {
	return &Request{
		ctx:    ctx,
		method: method,
		url:    url,
		body:   body,
		header: make(http.Header),
	}
}

// SetHeader sets a header on the request.
func (r *Request) SetHeader(key, value string) *Request {
	r.header.Set(key, value)
	return r
}

// Context returns the request context.
func (r *Request) Context() context.Context {
	return r.ctx
}

// buildHTTP creates a standard http.Request from this wrapper.
func (r *Request) buildHTTP() *http.Request {
	var bodyReader *bytes.Reader
	if r.body != nil {
		bodyReader = bytes.NewReader(r.body)
	}

	var httpReq *http.Request
	var err error
	if bodyReader != nil {
		httpReq, err = http.NewRequestWithContext(r.ctx, r.method, r.url, bodyReader)
	} else {
		httpReq, err = http.NewRequestWithContext(r.ctx, r.method, r.url, nil)
	}
	if err != nil {
		// Should not happen with valid methods/URLs.
		return nil
	}

	for k, v := range r.header {
		httpReq.Header[k] = v
	}

	return httpReq
}
