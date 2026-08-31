package http

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// Option configures an HTTP Client.
type Option func(*clientConfig)

// clientConfig holds the resolved configuration for a Client.
type clientConfig struct {
	baseURL             string
	timeout             time.Duration
	defaultHeaders      map[string]string
	tlsInsecureSkipVerify bool
	metricPrefix        string
	transport           http.RoundTripper
	maxIdleConns        int
	maxIdleConnsPerHost int
	idleConnTimeout     time.Duration
	detailedTiming      bool
	discardBody         bool
}

func defaultConfig() clientConfig {
	return clientConfig{
		timeout:             30 * time.Second,
		metricPrefix:        defaultMetricPrefix,
		maxIdleConns:        100,
		maxIdleConnsPerHost: 100,
		idleConnTimeout:     90 * time.Second,
	}
}

// WithBaseURL sets the base URL prepended to relative request paths.
func WithBaseURL(baseURL string) Option {
	return func(c *clientConfig) {
		c.baseURL = baseURL
	}
}

// WithTimeout sets the per-request timeout for the HTTP client.
func WithTimeout(d time.Duration) Option {
	return func(c *clientConfig) {
		c.timeout = d
	}
}

// WithHeader adds a default header that is included in every request made by this client.
func WithHeader(key, value string) Option {
	return func(c *clientConfig) {
		if c.defaultHeaders == nil {
			c.defaultHeaders = make(map[string]string)
		}
		c.defaultHeaders[key] = value
	}
}

// WithHeaders adds multiple default headers that are included in every request made by this client.
func WithHeaders(headers map[string]string) Option {
	return func(c *clientConfig) {
		if c.defaultHeaders == nil {
			c.defaultHeaders = make(map[string]string, len(headers))
		}
		for k, v := range headers {
			c.defaultHeaders[k] = v
		}
	}
}

// WithTLSInsecureSkipVerify disables TLS certificate verification.
// WARNING: This should only be used for testing against self-signed certificates.
func WithTLSInsecureSkipVerify() Option {
	return func(c *clientConfig) {
		c.tlsInsecureSkipVerify = true
	}
}

// WithCustomMetricPrefix overrides the default metric name prefix ("vuhive.http.").
// The prefix is prepended to metric suffixes like "req_duration", "req_failed", and "reqs".
func WithCustomMetricPrefix(prefix string) Option {
	return func(c *clientConfig) {
		c.metricPrefix = prefix
	}
}

// WithTransport provides a custom http.RoundTripper for the underlying HTTP client.
// When set, transport-related options (MaxIdleConns, TLS settings, etc.) are ignored.
func WithTransport(rt http.RoundTripper) Option {
	return func(c *clientConfig) {
		c.transport = rt
	}
}

// WithMaxIdleConns configures the maximum number of idle (keep-alive) connections across all hosts.
func WithMaxIdleConns(n int) Option {
	return func(c *clientConfig) {
		c.maxIdleConns = n
	}
}

// WithMaxIdleConnsPerHost configures the maximum idle connections to keep per host.
func WithMaxIdleConnsPerHost(n int) Option {
	return func(c *clientConfig) {
		c.maxIdleConnsPerHost = n
	}
}

// WithIdleConnTimeout configures how long idle connections remain in the pool before being closed.
func WithIdleConnTimeout(d time.Duration) Option {
	return func(c *clientConfig) {
		c.idleConnTimeout = d
	}
}

// WithDetailedTiming enables collection of detailed per-phase HTTP timing metrics
// (connecting, TLS handshaking, sending, receiving) using net/http/httptrace.
// These metrics are NOT collected by default to minimize overhead.
func WithDetailedTiming() Option {
	return func(c *clientConfig) {
		c.detailedTiming = true
	}
}

// WithDiscardBody configures the client to discard response bodies without reading them into memory.
// The Response.Body field will be nil. Status code, headers, and metrics are still recorded normally.
// This is a significant optimization for load test scenarios that only need to measure latency
// and status codes without inspecting response payloads.
func WithDiscardBody() Option {
	return func(c *clientConfig) {
		c.discardBody = true
	}
}

// buildTransport constructs an http.Transport from the resolved configuration.
// Returns nil if a custom transport was provided via WithTransport.
func (cfg *clientConfig) buildTransport() http.RoundTripper {
	if cfg.transport != nil {
		return cfg.transport
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          cfg.maxIdleConns,
		MaxIdleConnsPerHost:   cfg.maxIdleConnsPerHost,
		IdleConnTimeout:       cfg.idleConnTimeout,
		TLSHandshakeTimeout:  10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   cfg.timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	if cfg.tlsInsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // user opted in explicitly
		}
	}

	return transport
}
