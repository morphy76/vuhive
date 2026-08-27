package http

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SSEEvent represents a single Server-Sent Event frame.
type SSEEvent struct {
	ID    string // Optional event ID (`id: <id>`)
	Event string // Event type (`event: <type>`, defaults to "message")
	Data  string // Event payload (`data: <content>`, supports multi-line data)
	Retry int    // Reconnection time in milliseconds (`retry: <ms>`)
}

// StreamOption configures SSE streaming behavior.
type StreamOption func(*streamConfig)

type streamConfig struct {
	bufferSize int
}

func defaultStreamConfig() streamConfig {
	return streamConfig{
		bufferSize: 4096,
	}
}

// WithStreamBufferSize sets the buffer size for reading SSE stream chunks.
func WithStreamBufferSize(size int) StreamOption {
	return func(cfg *streamConfig) {
		if size > 0 {
			cfg.bufferSize = size
		}
	}
}

// SSEStream represents an active Server-Sent Events stream connection.
type SSEStream struct {
	StatusCode int
	Headers    http.Header

	client    *Client
	method    string
	metricURL string
	resp      *http.Response
	reader    *bufio.Reader
	ctx       context.Context
	cancel    context.CancelFunc

	startTime time.Time
	lastEvent time.Time

	current SSEEvent
	err     error

	closed atomic.Bool

	eventsChan chan SSEEvent
	eventsOnce sync.Once
}

// Next reads the next SSEEvent from the stream. Returns false on stream EOF or ctx cancellation.
func (s *SSEStream) Next() bool {
	if s.err != nil || s.closed.Load() {
		return false
	}

	var (
		dataBuilder strings.Builder
		eventType   string
		eventID     string
		retryMs     int
		hasData     bool
		hasField    bool
	)

	for {
		if s.ctx.Err() != nil {
			return false
		}

		line, readErr := s.reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			if errors.Is(s.ctx.Err(), context.Canceled) || errors.Is(s.ctx.Err(), context.DeadlineExceeded) {
				return false
			}
			s.err = readErr
			s.client.recordSSEError(s.method, s.metricURL)
			return false
		}

		trimmed := strings.TrimRight(line, "\r\n")

		if len(trimmed) == 0 {
			if hasField || hasData {
				s.dispatch(eventID, eventType, dataBuilder.String(), retryMs)
				return true
			}
			if readErr == io.EOF {
				return false
			}
			continue
		}

		if strings.HasPrefix(trimmed, ":") {
			// Comment line, ignore per W3C specification
			if readErr == io.EOF {
				if hasField || hasData {
					s.dispatch(eventID, eventType, dataBuilder.String(), retryMs)
					return true
				}
				return false
			}
			continue
		}

		colonIdx := strings.IndexByte(trimmed, ':')
		var field, value string
		if colonIdx >= 0 {
			field = trimmed[:colonIdx]
			value = strings.TrimPrefix(trimmed[colonIdx+1:], " ")
		} else {
			field = trimmed
			value = ""
		}

		switch field {
		case "event":
			eventType = value
			hasField = true
		case "data":
			if hasData {
				dataBuilder.WriteByte('\n')
			}
			dataBuilder.WriteString(value)
			hasData = true
			hasField = true
		case "id":
			eventID = value
			hasField = true
		case "retry":
			if n, err := strconv.Atoi(value); err == nil {
				retryMs = n
				hasField = true
			}
		default:
			// Unknown field, ignore per specification
		}

		if readErr == io.EOF {
			if hasField || hasData {
				s.dispatch(eventID, eventType, dataBuilder.String(), retryMs)
				return true
			}
			return false
		}
	}
}

func (s *SSEStream) dispatch(id, event, data string, retry int) {
	if event == "" {
		event = "message"
	}
	s.current = SSEEvent{
		ID:    id,
		Event: event,
		Data:  data,
		Retry: retry,
	}

	now := time.Now()
	var latency time.Duration
	if !s.lastEvent.IsZero() {
		latency = now.Sub(s.lastEvent)
	} else {
		latency = now.Sub(s.startTime)
	}
	s.lastEvent = now

	s.client.recordSSEEvent(s.method, s.metricURL, s.current.Event, latency)
}

// Event returns the most recently decoded event.
func (s *SSEStream) Event() SSEEvent {
	return s.current
}

// Err returns any non-EOF error encountered during streaming.
func (s *SSEStream) Err() error {
	return s.err
}

// Close terminates the streaming connection and releases underlying network resources.
func (s *SSEStream) Close() error {
	if s.closed.Swap(true) {
		return nil
	}

	if s.cancel != nil {
		s.cancel()
	}

	var closeErr error
	if s.resp != nil && s.resp.Body != nil {
		closeErr = s.resp.Body.Close()
	}

	duration := time.Since(s.startTime)
	s.client.recordSSEStreamDuration(s.method, s.metricURL, s.StatusCode, duration)

	return closeErr
}

// Events returns a read-only channel of events for select-loop concurrency.
func (s *SSEStream) Events() <-chan SSEEvent {
	s.eventsOnce.Do(func() {
		s.eventsChan = make(chan SSEEvent, 16)
		go func() {
			defer close(s.eventsChan)
			for s.Next() {
				select {
				case s.eventsChan <- s.Event():
				case <-s.ctx.Done():
					return
				}
			}
		}()
	})
	return s.eventsChan
}

// StreamSSE establishes an HTTP GET connection requesting text/event-stream and returns an SSEStream.
func (c *Client) StreamSSE(ctx context.Context, rawURL string, opts ...StreamOption) (*SSEStream, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.resolveURL(rawURL), nil)
	if err != nil {
		return nil, fmt.Errorf("vuhive/http: failed to create SSE request: %w", err)
	}
	return c.DoStream(ctx, req, opts...)
}

// DoStream executes a custom request and wraps the response body in an SSEStream.
//
// Context resolution follows a precedence chain:
//  1. If ctx is non-nil and not context.Background(), use ctx.
//  2. Else if req is non-nil and req.Context() is non-nil and not context.Background(), use req.Context().
//  3. Else default to context.Background().
//
// To support long-lived SSE streams (e.g. LLM token streaming, live event feeds),
// DoStream detaches short parent iteration deadlines (vu_timeout) using
// context.WithoutCancel, while a monitoring goroutine ensures the stream is
// cleanly terminated when the parent context is explicitly canceled (e.g.
// scenario teardown or VU cancellation).
func (c *Client) DoStream(ctx context.Context, req *http.Request, opts ...StreamOption) (*SSEStream, error) {
	strCfg := defaultStreamConfig()
	for _, opt := range opts {
		opt(&strCfg)
	}

	// --- Context precedence & fallback ---
	effectiveCtx := resolveEffectiveContext(ctx, req)

	// --- Pre-flight check: reject already-canceled context ---
	if effectiveCtx.Err() != nil {
		return nil, fmt.Errorf("vuhive/http: SSE stream context already canceled: %w", effectiveCtx.Err())
	}

	if req == nil {
		return nil, fmt.Errorf("vuhive/http: SSE stream request must not be nil")
	}

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

	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "text/event-stream")
	}

	method := req.Method
	metricURL := sanitizeURL(req.URL)

	// --- Deadline detachment for long-lived streams ---
	// Strip short parent iteration deadlines (vu_timeout) so they don't kill
	// persistent SSE connections prematurely, while preserving values stored
	// in the parent context.
	detachedCtx := context.WithoutCancel(effectiveCtx)
	streamCtx, streamCancel := context.WithCancel(detachedCtx)

	if req.Context() != streamCtx {
		req = req.WithContext(streamCtx)
	}

	// Streaming client without overall lifecycle timeout so long-lived streams are not prematurely killed
	streamingClient := &http.Client{
		Transport:     c.inner.Transport,
		CheckRedirect: c.inner.CheckRedirect,
		Jar:           c.inner.Jar,
		Timeout:       0,
	}

	start := time.Now()
	resp, err := streamingClient.Do(req)
	connectDuration := time.Since(start)

	if err != nil {
		streamCancel()
		c.recordSSEConnectFailed(method, metricURL, connectDuration)
		return nil, fmt.Errorf("vuhive/http: SSE stream connection failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		streamCancel()
		c.recordSSEConnect(method, metricURL, resp.StatusCode, connectDuration)
		c.recordSSEError(method, metricURL)
		return nil, fmt.Errorf("vuhive/http: SSE stream unexpected status code: %d", resp.StatusCode)
	}

	c.recordSSEConnect(method, metricURL, resp.StatusCode, connectDuration)

	stream := &SSEStream{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		client:     c,
		method:     method,
		metricURL:  metricURL,
		resp:       resp,
		reader:     bufio.NewReaderSize(resp.Body, strCfg.bufferSize),
		ctx:        streamCtx,
		cancel:     streamCancel,
		startTime:  start,
		lastEvent:  time.Now(),
	}

	// --- Cancellation monitoring goroutine ---
	// Monitor the parent context for explicit cancellation (context.Canceled).
	// Deadline expiration (context.DeadlineExceeded) is intentionally ignored
	// to allow long-lived streams to survive short iteration timeouts.
	go func() {
		select {
		case <-effectiveCtx.Done():
			if errors.Is(effectiveCtx.Err(), context.Canceled) {
				streamCancel()
			}
			// DeadlineExceeded: intentionally do NOT cancel the stream.
		case <-streamCtx.Done():
			// Stream was closed independently (e.g. via stream.Close()).
		}
	}()

	return stream, nil
}

// resolveEffectiveContext determines the best available context for stream lifecycle.
func resolveEffectiveContext(ctx context.Context, req *http.Request) context.Context {
	if ctx != nil && ctx != context.Background() {
		return ctx
	}
	if req != nil {
		if reqCtx := req.Context(); reqCtx != nil && reqCtx != context.Background() {
			return reqCtx
		}
	}
	return context.Background()
}

// isSSEAcceptHeader returns true if the Accept header indicates an SSE stream request.
func isSSEAcceptHeader(accept string) bool {
	return strings.Contains(strings.ToLower(accept), "text/event-stream")
}

var _ io.Closer = (*SSEStream)(nil)
