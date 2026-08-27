package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"time"
)

// bodyBufferPool is a sync.Pool of *bytes.Buffer used to reuse memory when reading
// HTTP response bodies, reducing GC pressure in high-throughput load testing scenarios.
var bodyBufferPool = sync.Pool{
	New: func() any { return bytes.NewBuffer(make([]byte, 0, 4096)) },
}

// resolveURL resolves rawURL against cfg.baseURL if cfg.baseURL is configured and rawURL is relative.
func (c *Client) resolveURL(rawURL string) string {
	if c.cfg.baseURL == "" {
		return rawURL
	}
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}
	base := strings.TrimRight(c.cfg.baseURL, "/")
	path := strings.TrimLeft(rawURL, "/")
	return base + "/" + path
}

// Get performs an HTTP GET request and returns an instrumented Response.
// Metrics are automatically recorded including latency, request count, and failure rate.
func (c *Client) Get(ctx context.Context, rawURL string) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.resolveURL(rawURL), nil)
	if err != nil {
		return nil, fmt.Errorf("vuhive/http: failed to create GET request: %w", err)
	}
	return c.Do(ctx, req)
}

// Post performs an HTTP POST request with the given content type and body.
// Metrics are automatically recorded including latency, request count, and failure rate.
func (c *Client) Post(ctx context.Context, rawURL string, contentType string, body io.Reader) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.resolveURL(rawURL), body)
	if err != nil {
		return nil, fmt.Errorf("vuhive/http: failed to create POST request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(ctx, req)
}

// Put performs an HTTP PUT request with the given content type and body.
// Metrics are automatically recorded including latency, request count, and failure rate.
func (c *Client) Put(ctx context.Context, rawURL string, contentType string, body io.Reader) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.resolveURL(rawURL), body)
	if err != nil {
		return nil, fmt.Errorf("vuhive/http: failed to create PUT request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(ctx, req)
}

// Delete performs an HTTP DELETE request.
// Metrics are automatically recorded including latency, request count, and failure rate.
func (c *Client) Delete(ctx context.Context, rawURL string) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.resolveURL(rawURL), nil)
	if err != nil {
		return nil, fmt.Errorf("vuhive/http: failed to create DELETE request: %w", err)
	}
	return c.Do(ctx, req)
}

// Do executes an arbitrary HTTP request and returns an instrumented Response.
// Default headers configured via WithHeader/WithHeaders are added to the request.
// Metrics are automatically recorded for every call.
func (c *Client) Do(ctx context.Context, req *http.Request) (*Response, error) {
	if req == nil {
		return nil, fmt.Errorf("vuhive/http: request must not be nil")
	}

	effectiveCtx := resolveEffectiveContext(ctx, req)

	if c.cfg.baseURL != "" && req.URL != nil && (req.URL.Scheme == "" || req.URL.Host == "") {
		resolved, err := url.Parse(c.resolveURL(req.URL.String()))
		if err == nil {
			req.URL = resolved
		}
	}

	// Apply default headers (do not overwrite headers already set on the request).
	for k, v := range c.cfg.defaultHeaders {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}

	method := req.Method
	metricURL := sanitizeURL(req.URL)

	var timings traceTimings
	if c.cfg.detailedTiming {
		traceCtx := httptrace.WithClientTrace(effectiveCtx, newClientTrace(&timings))
		req = req.WithContext(traceCtx)
	} else if req.Context() != effectiveCtx {
		req = req.WithContext(effectiveCtx)
	}

	start := time.Now()
	resp, err := c.inner.Do(req)

	totalDuration := time.Since(start)

	if err != nil {
		c.recordFailedMetrics(method, metricURL, totalDuration)
		return nil, fmt.Errorf("vuhive/http: request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var bodyBytes []byte
	if c.cfg.discardBody {
		_, readErr := io.Copy(io.Discard, resp.Body)
		readDone := time.Now()
		if readErr != nil {
			c.recordFailedMetrics(method, metricURL, time.Since(start))
			return nil, fmt.Errorf("vuhive/http: failed to drain response body: %w", readErr)
		}
		failed := resp.StatusCode < 200 || resp.StatusCode >= 400
		c.recordMetrics(method, metricURL, resp.StatusCode, totalDuration, failed)
		if c.cfg.detailedTiming {
			c.recordDetailedTimingsFromTrace(method, metricURL, resp.StatusCode, &timings, readDone)
		}
	} else {
		buf := bodyBufferPool.Get().(*bytes.Buffer)
		buf.Reset()
		_, readErr := io.Copy(buf, resp.Body)
		readDone := time.Now()
		if readErr != nil {
			bodyBufferPool.Put(buf)
			c.recordFailedMetrics(method, metricURL, time.Since(start))
			return nil, fmt.Errorf("vuhive/http: failed to read response body: %w", readErr)
		}
		bodyBytes = make([]byte, buf.Len())
		copy(bodyBytes, buf.Bytes())
		bodyBufferPool.Put(buf)

		failed := resp.StatusCode < 200 || resp.StatusCode >= 400
		c.recordMetrics(method, metricURL, resp.StatusCode, totalDuration, failed)
		if c.cfg.detailedTiming {
			c.recordDetailedTimingsFromTrace(method, metricURL, resp.StatusCode, &timings, readDone)
		}
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       bodyBytes,
	}, nil
}

// sanitizeURL extracts the path from a URL for metric tagging,
// stripping query parameters and fragments to prevent high-cardinality tags.
func sanitizeURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	return u.Path
}
