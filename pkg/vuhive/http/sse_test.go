package http_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/morphy76/vuhive/pkg/vuhive"
	vuhivehttp "github.com/morphy76/vuhive/pkg/vuhive/http"
)

func TestSSE_Parser_StandardEvents(t *testing.T) {
	rawStream := "id: 101\nevent: user_login\ndata: {\"user_id\":\"u1\"}\nretry: 3000\n\n" +
		"event: update\ndata: hello world\n\n"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "text/event-stream", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(rawStream))
	}))
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	stream, err := client.StreamSSE(context.Background(), ts.URL+"/events")
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	// First event
	require.True(t, stream.Next(), "expected first event")
	ev1 := stream.Event()
	assert.Equal(t, "101", ev1.ID)
	assert.Equal(t, "user_login", ev1.Event)
	assert.Equal(t, `{"user_id":"u1"}`, ev1.Data)
	assert.Equal(t, 3000, ev1.Retry)

	// Second event
	require.True(t, stream.Next(), "expected second event")
	ev2 := stream.Event()
	assert.Equal(t, "", ev2.ID)
	assert.Equal(t, "update", ev2.Event)
	assert.Equal(t, "hello world", ev2.Data)

	// End of stream
	assert.False(t, stream.Next(), "expected EOF")
	assert.NoError(t, stream.Err())
}

func TestSSE_Parser_MultiLineData(t *testing.T) {
	rawStream := "data: line 1\r\ndata: line 2\r\ndata: line 3\r\n\r\n"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(rawStream))
	}))
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	stream, err := client.StreamSSE(context.Background(), ts.URL+"/multiline")
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	require.True(t, stream.Next())
	ev := stream.Event()
	assert.Equal(t, "message", ev.Event, "default event type should be message")
	assert.Equal(t, "line 1\nline 2\nline 3", ev.Data)

	assert.False(t, stream.Next())
	assert.NoError(t, stream.Err())
}

func TestSSE_Parser_CommentsAndIgnoredFields(t *testing.T) {
	rawStream := ": heartbeat comment\n" +
		"custom_field: some_value\n" +
		"data: actual payload\n\n"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(rawStream))
	}))
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	stream, err := client.StreamSSE(context.Background(), ts.URL+"/comments")
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	require.True(t, stream.Next())
	ev := stream.Event()
	assert.Equal(t, "actual payload", ev.Data)
	assert.Equal(t, "message", ev.Event)

	assert.False(t, stream.Next())
	assert.NoError(t, stream.Err())
}

func TestSSE_Client_StreamConsumption(t *testing.T) {
	eventCount := 5
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		for i := 1; i <= eventCount; i++ {
			_, _ = fmt.Fprintf(w, "id: %d\nevent: token\ndata: tok_%d\n\n", i, i)
			flusher.Flush()
			time.Sleep(5 * time.Millisecond)
		}
	}))
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	stream, err := client.StreamSSE(context.Background(), ts.URL+"/tokens")
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	var received []vuhivehttp.SSEEvent
	for stream.Next() {
		received = append(received, stream.Event())
	}
	require.NoError(t, stream.Err())
	assert.Equal(t, eventCount, len(received))

	for i, ev := range received {
		assert.Equal(t, fmt.Sprintf("%d", i+1), ev.ID)
		assert.Equal(t, "token", ev.Event)
		assert.Equal(t, fmt.Sprintf("tok_%d", i+1), ev.Data)
	}
}

func TestSSE_Client_EventsChannel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		for i := 1; i <= 3; i++ {
			_, _ = fmt.Fprintf(w, "data: event_%d\n\n", i)
			flusher.Flush()
		}
	}))
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	stream, err := client.StreamSSE(context.Background(), ts.URL+"/chan")
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	var collected []string
	for ev := range stream.Events() {
		collected = append(collected, ev.Data)
	}

	assert.Equal(t, []string{"event_1", "event_2", "event_3"}, collected)
	assert.NoError(t, stream.Err())
}

func TestSSE_Client_ContextCancellation(t *testing.T) {
	serverClosed := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		_, _ = fmt.Fprintf(w, "data: initial\n\n")
		flusher.Flush()

		<-r.Context().Done()
		close(serverClosed)
	}))
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := client.StreamSSE(ctx, ts.URL+"/cancel")
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	require.True(t, stream.Next())
	assert.Equal(t, "initial", stream.Event().Data)

	// Cancel context while stream is waiting for next event
	cancel()

	assert.False(t, stream.Next(), "Next should return false after context cancel")

	select {
	case <-serverClosed:
		// Server observed context cancellation
	case <-time.After(1 * time.Second):
		t.Fatal("server did not observe context cancellation within 1s")
	}
}

func TestSSE_Client_MetricsRecorded(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		for i := 1; i <= 4; i++ {
			_, _ = fmt.Fprintf(w, "event: chunk\ndata: data_%d\n\n", i)
			flusher.Flush()
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer ts.Close()

	store, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	stream, err := client.StreamSSE(context.Background(), ts.URL+"/metrics")
	require.NoError(t, err)

	for stream.Next() {
		_ = stream.Event()
	}
	require.NoError(t, stream.Err())
	require.NoError(t, stream.Close())

	// 1. connections_total
	connCount := store.AggregatedCounterValue(vuhive.MetricHTTPSSEConnectionsTotal)
	assert.Equal(t, int64(1), connCount, "connections_total should be 1")

	// 2. connect_duration
	connDurationSnap := store.MergedHistogramSnapshot(vuhive.MetricHTTPSSEConnectDuration)
	assert.Equal(t, int64(1), connDurationSnap.Count, "connect_duration should be recorded")

	// 3. events_total
	eventsCount := store.AggregatedCounterValue(vuhive.MetricHTTPSSEEventsTotal)
	assert.Equal(t, int64(4), eventsCount, "events_total should be 4")

	// 4. event_latency
	eventLatencySnap := store.MergedHistogramSnapshot(vuhive.MetricHTTPSSEEventLatency)
	assert.Equal(t, int64(4), eventLatencySnap.Count, "event_latency should be recorded for each event")

	// 5. stream_duration
	streamDurSnap := store.MergedHistogramSnapshot(vuhive.MetricHTTPSSEStreamDuration)
	assert.Equal(t, int64(1), streamDurSnap.Count, "stream_duration should be recorded on close")

	// 6. errors_total should be 0
	errCount := store.AggregatedCounterValue(vuhive.MetricHTTPSSEErrorsTotal)
	assert.Equal(t, int64(0), errCount, "errors_total should be 0 for clean stream")
}

func TestSSE_Client_Non2xxResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Not Found"))
	}))
	defer ts.Close()

	store, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	stream, err := client.StreamSSE(context.Background(), ts.URL+"/missing")
	assert.Error(t, err, "StreamSSE should return error for non-2xx response")
	assert.Nil(t, stream)

	// Verify error metric recorded
	errCount := store.AggregatedCounterValue(vuhive.MetricHTTPSSEErrorsTotal)
	assert.Equal(t, int64(1), errCount, "errors_total should be recorded on non-2xx")
}

func TestSSE_Client_DoStream_CustomMethodAndBody(t *testing.T) {
	var capturedMethod, capturedBody, capturedAccept string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedAccept = r.Header.Get("Accept")
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		capturedBody = string(buf[:n])

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: response\n\n"))
	}))
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/generate", strings.NewReader(`{"prompt":"hello"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	stream, err := client.DoStream(context.Background(), req)
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	assert.Equal(t, http.MethodPost, capturedMethod)
	assert.Equal(t, "text/event-stream", capturedAccept)
	assert.Equal(t, `{"prompt":"hello"}`, capturedBody)

	require.True(t, stream.Next())
	assert.Equal(t, "response", stream.Event().Data)
	assert.False(t, stream.Next())
	assert.NoError(t, stream.Err())
}

func TestSSE_DoStream_NilContext_FallsBackToReqContext(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: ok\n\n"))
	}))
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/nil-ctx", nil)
	require.NoError(t, err)

	//nolint:staticcheck // SA1012: intentionally passing nil ctx to test fallback behavior
	stream, err := client.DoStream(nil, req)
	require.NoError(t, err, "DoStream should not panic or fail when ctx is nil")
	defer func() { _ = stream.Close() }()

	require.True(t, stream.Next())
	assert.Equal(t, "ok", stream.Event().Data)
	assert.False(t, stream.Next())
	assert.NoError(t, stream.Err())
}

func TestSSE_DoStream_BackgroundContext_FallsBackToReqContext(t *testing.T) {
	reqCtx, reqCancel := context.WithCancel(context.Background())
	defer reqCancel()

	serverClosed := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		_, _ = fmt.Fprintf(w, "data: initial\n\n")
		flusher.Flush()

		<-r.Context().Done()
		close(serverClosed)
	}))
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, ts.URL+"/bg-ctx", nil)
	require.NoError(t, err)

	// Pass context.Background() as ctx; DoStream should fall back to req.Context() (reqCtx).
	stream, err := client.DoStream(context.Background(), req)
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	require.True(t, stream.Next())
	assert.Equal(t, "initial", stream.Event().Data)

	// Cancel reqCtx — should eventually tear down the stream since it was used as effective context.
	reqCancel()

	assert.False(t, stream.Next(), "Next should return false after reqCtx cancel")

	select {
	case <-serverClosed:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not observe context cancellation within 2s")
	}
}

func TestSSE_DoStream_PreCanceledContext_ReturnsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: should_not_reach\n\n"))
	}))
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	stream, err := client.DoStream(ctx, nil)
	assert.Error(t, err, "DoStream should return error for pre-canceled context")
	assert.Nil(t, stream)
	assert.Contains(t, err.Error(), "context", "error should reference context cancellation")
}

func TestSSE_DoStream_DeadlineDoesNotKillStream(t *testing.T) {
	// Core bug fix test: A short parent deadline must NOT kill a long-lived SSE stream.
	eventCount := 5
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		for i := 1; i <= eventCount; i++ {
			_, _ = fmt.Fprintf(w, "data: event_%d\n\n", i)
			flusher.Flush()
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	// Create a parent context with a very short deadline (100ms).
	// The stream will take ~250ms to deliver all 5 events.
	// Before this fix, the stream would be killed after 100ms.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	stream, err := client.StreamSSE(ctx, ts.URL+"/deadline-test")
	require.NoError(t, err, "StreamSSE should connect before deadline expires")
	defer func() { _ = stream.Close() }()

	var received []string
	for stream.Next() {
		received = append(received, stream.Event().Data)
	}

	require.NoError(t, stream.Err(), "stream should complete without error")
	assert.Equal(t, eventCount, len(received), "all events should be received despite short parent deadline")
	for i, data := range received {
		assert.Equal(t, fmt.Sprintf("event_%d", i+1), data)
	}
}

func TestSSE_DoStream_ExplicitCancelKillsStream(t *testing.T) {
	// Explicit parent cancel (not deadline) should still cleanly terminate the stream.
	serverClosed := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		_, _ = fmt.Fprintf(w, "data: first\n\n")
		flusher.Flush()

		<-r.Context().Done()
		close(serverClosed)
	}))
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := client.StreamSSE(ctx, ts.URL+"/explicit-cancel")
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	require.True(t, stream.Next())
	assert.Equal(t, "first", stream.Event().Data)

	// Explicit cancel — should terminate the stream.
	cancel()

	assert.False(t, stream.Next(), "Next should return false after explicit cancel")

	select {
	case <-serverClosed:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not observe explicit context cancellation within 2s")
	}
}
