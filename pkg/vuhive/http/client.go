package http

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/morphy76/vuhive/pkg/vuhive"
)

var (
	defaultClientsMu sync.RWMutex
	defaultClients   = make(map[any]*Client)
)

// ContextProvider provides the configuration and metrics capabilities required
// to construct or retrieve a default scenario HTTP client.
// Both vuhive.SetupContext and vuhive.VUContext satisfy this interface.
type ContextProvider interface {
	HTTPConfig() vuhive.HTTPConfig
	Metrics() vuhive.MetricsCollector
}

// Default returns a shared, instrumented HTTP client initialized from the scenario's
// declarative HTTP configuration in vuhive.yaml.
// The client is constructed lazily upon first call and cached as a shared singleton
// for the scenario execution, safe for concurrent use across all VUs.
func Default(ctx ContextProvider) *Client {
	var key any
	if ident, ok := ctx.(vuhive.ExecutionIdentity); ok && ident.ScenarioName() != "" {
		key = ident.ScenarioName()
	} else if ctx.Metrics() != nil {
		key = ctx.Metrics()
	} else {
		key = "default"
	}

	defaultClientsMu.RLock()
	client, ok := defaultClients[key]
	defaultClientsMu.RUnlock()
	if ok {
		return client
	}

	defaultClientsMu.Lock()
	defer defaultClientsMu.Unlock()
	if client, ok = defaultClients[key]; ok {
		return client
	}

	client = newClientFromHTTPConfig(ctx.HTTPConfig(), ctx.Metrics())
	defaultClients[key] = client
	return client
}

// Client is an instrumented HTTP client that automatically records latency,
// status code counters, and error rates for every request executed.
// Client is safe for concurrent use from multiple VU goroutines.
type Client struct {
	inner   *http.Client
	cfg     clientConfig
	metrics vuhive.MetricsCollector
}

// BaseURL returns the configured base URL for this client.
func (c *Client) BaseURL() string {
	return c.cfg.baseURL
}

// NewClient creates an instrumented HTTP client from a SetupContext.
// The MetricsCollector is extracted from the context for automatic metric recording.
// Options configure timeouts, headers, TLS settings, and transport parameters.
func NewClient(ctx vuhive.SetupContext, opts ...Option) *Client {
	return newClientWithMetrics(ctx.Metrics(), opts...)
}

// NewClientFromVU creates an instrumented HTTP client from a VUContext.
// Use this when you need a per-VU client instance rather than a shared client created in Setup.
func NewClientFromVU(ctx vuhive.VUContext, opts ...Option) *Client {
	return newClientWithMetrics(ctx.Metrics(), opts...)
}

// NewClientFromConfig creates an instrumented HTTP client initialized from the SetupContext's
// declarative HTTP configuration (vuhive.yaml), with optional programmatic Option overrides.
func NewClientFromConfig(ctx vuhive.SetupContext, opts ...Option) *Client {
	return newClientFromHTTPConfig(ctx.HTTPConfig(), ctx.Metrics(), opts...)
}

// NewClientFromVUConfig creates an instrumented HTTP client initialized from the VUContext's
// declarative HTTP configuration (vuhive.yaml), with optional programmatic Option overrides.
func NewClientFromVUConfig(ctx vuhive.VUContext, opts ...Option) *Client {
	return newClientFromHTTPConfig(ctx.HTTPConfig(), ctx.Metrics(), opts...)
}

func newClientFromHTTPConfig(httpCfg vuhive.HTTPConfig, metrics vuhive.MetricsCollector, opts ...Option) *Client {
	cfg := defaultConfig()
	if httpCfg.BaseURL != "" {
		cfg.baseURL = httpCfg.BaseURL
	}
	if httpCfg.Timeout > 0 {
		cfg.timeout = httpCfg.Timeout
	}
	if len(httpCfg.Headers) > 0 {
		if cfg.defaultHeaders == nil {
			cfg.defaultHeaders = make(map[string]string, len(httpCfg.Headers))
		}
		for k, v := range httpCfg.Headers {
			cfg.defaultHeaders[k] = v
		}
	}
	if httpCfg.TLS.InsecureSkipVerify {
		cfg.tlsInsecureSkipVerify = true
	}
	if httpCfg.Pool.MaxIdleConns > 0 {
		cfg.maxIdleConns = httpCfg.Pool.MaxIdleConns
	}
	if httpCfg.Pool.MaxIdleConnsPerHost > 0 {
		cfg.maxIdleConnsPerHost = httpCfg.Pool.MaxIdleConnsPerHost
	}
	if httpCfg.Pool.IdleConnTimeout > 0 {
		cfg.idleConnTimeout = httpCfg.Pool.IdleConnTimeout
	}
	if httpCfg.DetailedTiming {
		cfg.detailedTiming = true
	}
	if httpCfg.MetricPrefix != "" {
		cfg.metricPrefix = httpCfg.MetricPrefix
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return &Client{
		inner: &http.Client{
			Timeout:   cfg.timeout,
			Transport: cfg.buildTransport(),
		},
		cfg:     cfg,
		metrics: metrics,
	}
}

// NewClientWithCollector creates an instrumented HTTP client from a MetricsCollector directly.
// This is primarily useful for testing or for constructing a client outside of a scenario context.
func NewClientWithCollector(metrics vuhive.MetricsCollector, opts ...Option) *Client {
	return newClientWithMetrics(metrics, opts...)
}

func newClientWithMetrics(metrics vuhive.MetricsCollector, opts ...Option) *Client {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	return &Client{
		inner: &http.Client{
			Timeout:   cfg.timeout,
			Transport: cfg.buildTransport(),
		},
		cfg:     cfg,
		metrics: metrics,
	}
}

// Transport returns an http.RoundTripper that automatically routes requests through this instrumented Client.
// Requests with an 'Accept' header containing 'text/event-stream' are transparently executed via DoStream
// and piped into standard HTTP streaming response bodies. All other requests are executed via Do.
func (c *Client) Transport() http.RoundTripper {
	return &standardTransport{client: c}
}

// StandardClient returns a standard *http.Client backed by this instrumented Client.
// Use this to seamlessly integrate third-party SDKs (such as OpenAI, Anthropic, or AIW) that accept
// standard *http.Client instances while preserving full vuhive metrics recording and streaming support.
func (c *Client) StandardClient() *http.Client {
	return &http.Client{
		Transport: c.Transport(),
	}
}

type standardTransport struct {
	client *Client
}

func (t *standardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	acceptHeader := req.Header.Get("Accept")

	if isSSEAcceptHeader(acceptHeader) {
		stream, err := t.client.DoStream(ctx, req)
		if err != nil {
			return nil, err
		}

		pr, pw := io.Pipe()
		go func() {
			defer func() {
				_ = stream.Close()
			}()
			for stream.Next() {
				ev := stream.Event()
				var buf bytes.Buffer
				if ev.ID != "" {
					buf.WriteString("id: " + ev.ID + "\n")
				}
				if ev.Event != "" && ev.Event != "message" {
					buf.WriteString("event: " + ev.Event + "\n")
				}
				if ev.Retry > 0 {
					buf.WriteString(fmt.Sprintf("retry: %d\n", ev.Retry))
				}
				for _, line := range strings.Split(ev.Data, "\n") {
					buf.WriteString("data: " + line + "\n")
				}
				buf.WriteString("\n")
				if _, writeErr := pw.Write(buf.Bytes()); writeErr != nil {
					_ = pw.CloseWithError(writeErr)
					return
				}
			}
			if err := stream.Err(); err != nil {
				_ = pw.CloseWithError(err)
			} else {
				_ = pw.Close()
			}
		}()

		return &http.Response{
			StatusCode: stream.StatusCode,
			Status:     fmt.Sprintf("%d %s", stream.StatusCode, http.StatusText(stream.StatusCode)),
			Header:     stream.Headers.Clone(),
			Body:       &pipeStreamBody{PipeReader: pr, stream: stream},
			Request:    req,
		}, nil
	}

	resp, err := t.client.Do(ctx, req)
	if err != nil {
		return nil, err
	}

	return &http.Response{
		StatusCode:    resp.StatusCode,
		Status:        fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode)),
		Header:        resp.Headers.Clone(),
		Body:          io.NopCloser(bytes.NewReader(resp.Body)),
		ContentLength: int64(len(resp.Body)),
		Request:       req,
	}, nil
}

type pipeStreamBody struct {
	*io.PipeReader
	stream *SSEStream
}

func (b *pipeStreamBody) Close() error {
	streamErr := b.stream.Close()
	pipeErr := b.PipeReader.Close()
	if streamErr != nil {
		return streamErr
	}
	return pipeErr
}

var (
	_ http.RoundTripper = (*standardTransport)(nil)
	_ io.ReadCloser     = (*pipeStreamBody)(nil)
)
