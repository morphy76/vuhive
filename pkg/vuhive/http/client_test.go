package http_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/morphy76/vuhive/internal/metric"
	"github.com/morphy76/vuhive/pkg/vuhive"
	vuhivehttp "github.com/morphy76/vuhive/pkg/vuhive/http"
)

// metricsAdapter wraps internal metric.Collector to satisfy vuhive.MetricsCollector.
type metricsAdapter struct {
	collector metric.Collector
}

func (m *metricsAdapter) Counter(name string, tags vuhive.Tags) vuhive.Counter {
	return m.collector.Counter(name, metric.Tags(tags))
}

func (m *metricsAdapter) Gauge(name string, tags vuhive.Tags) vuhive.Gauge {
	return m.collector.Gauge(name, metric.Tags(tags))
}

func (m *metricsAdapter) Duration(name string, tags vuhive.Tags) vuhive.Duration {
	return m.collector.Duration(name, metric.Tags(tags))
}

func (m *metricsAdapter) Rate(name string, tags vuhive.Tags) vuhive.Rate {
	return m.collector.Rate(name, metric.Tags(tags))
}

// newTestStore creates a metric store and a MetricsCollector adapter for testing.
func newTestStore(t *testing.T) (*metric.Store, vuhive.MetricsCollector) {
	t.Helper()
	store := metric.NewStore()
	adapter := &metricsAdapter{collector: store}
	return store, adapter
}

// newTestServer creates a simple httptest.Server that returns JSON.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Echo-Method", r.Method)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
}

// newErrorServer creates a server that returns 500.
func newErrorServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
	}))
}

// --- Client constructor tests ---

func TestNewClient_DefaultOptions(t *testing.T) {
	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)
	assert.NotNil(t, client, "client should not be nil")
}

func TestNewClient_WithTimeout(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics, vuhivehttp.WithTimeout(5*time.Second))

	resp, err := client.Get(context.Background(), ts.URL+"/test")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestNewClient_WithHeaders(t *testing.T) {
	var capturedAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics,
		vuhivehttp.WithHeader("Authorization", "Bearer test-token"),
	)

	_, err := client.Get(context.Background(), ts.URL+"/test")
	require.NoError(t, err)
	assert.Equal(t, "Bearer test-token", capturedAuth)
}

func TestNewClient_WithBulkHeaders(t *testing.T) {
	var capturedAuth, capturedAccept string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics,
		vuhivehttp.WithHeaders(map[string]string{
			"Authorization": "Bearer bulk-token",
			"Accept":        "application/json",
		}),
	)

	_, err := client.Get(context.Background(), ts.URL+"/test")
	require.NoError(t, err)
	assert.Equal(t, "Bearer bulk-token", capturedAuth)
	assert.Equal(t, "application/json", capturedAccept)
}

func TestNewClient_WithHeaders_DoNotOverwriteExplicit(t *testing.T) {
	var capturedAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics,
		vuhivehttp.WithHeader("Authorization", "Bearer default"),
	)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/test", nil)
	req.Header.Set("Authorization", "Bearer explicit")
	_, err := client.Do(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "Bearer explicit", capturedAuth, "explicit header should not be overwritten")
}

func TestNewClient_WithCustomMetricPrefix(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	store, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics,
		vuhivehttp.WithCustomMetricPrefix("custom."),
	)

	_, err := client.Get(context.Background(), ts.URL+"/test")
	require.NoError(t, err)

	counterVal := store.AggregatedCounterValue("custom.reqs")
	assert.Equal(t, int64(1), counterVal, "custom prefix counter should be incremented")
}

func TestNewClient_WithTLSInsecureSkipVerify(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	_, metrics := newTestStore(t)

	// Without InsecureSkipVerify, HTTPS to a test server with self-signed cert should fail
	clientStrict := vuhivehttp.NewClientWithCollector(metrics)
	_, err := clientStrict.Get(context.Background(), ts.URL+"/test")
	assert.Error(t, err, "strict TLS should reject self-signed cert")

	// With InsecureSkipVerify, it should succeed
	clientInsecure := vuhivehttp.NewClientWithCollector(metrics, vuhivehttp.WithTLSInsecureSkipVerify())
	resp, err := clientInsecure.Get(context.Background(), ts.URL+"/test")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// --- Request method tests ---

func TestClient_Get_RecordsMetrics(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	store, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	resp, err := client.Get(context.Background(), ts.URL+"/api/test")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify request counter
	counterVal := store.AggregatedCounterValue(vuhive.MetricHTTPReqs)
	assert.Equal(t, int64(1), counterVal, "request counter should be 1")

	// Verify duration was recorded
	snap := store.MergedHistogramSnapshot(vuhive.MetricHTTPReqDuration)
	assert.Equal(t, int64(1), snap.Count, "duration should have 1 observation")
}

func TestClient_Post_RecordsMetrics(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	store, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	body := strings.NewReader(`{"item":"test"}`)
	resp, err := client.Post(context.Background(), ts.URL+"/api/items", "application/json", body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	counterVal := store.AggregatedCounterValue(vuhive.MetricHTTPReqs)
	assert.Equal(t, int64(1), counterVal)
}

func TestClient_Put_RecordsMetrics(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	store, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	body := strings.NewReader(`{"item":"updated"}`)
	resp, err := client.Put(context.Background(), ts.URL+"/api/items/1", "application/json", body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	counterVal := store.AggregatedCounterValue(vuhive.MetricHTTPReqs)
	assert.Equal(t, int64(1), counterVal)
}

func TestClient_Delete_RecordsMetrics(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	store, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	resp, err := client.Delete(context.Background(), ts.URL+"/api/items/1")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	counterVal := store.AggregatedCounterValue(vuhive.MetricHTTPReqs)
	assert.Equal(t, int64(1), counterVal)
}

func TestClient_Do_RecordsMetrics(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	store, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPatch, ts.URL+"/api/items/1", nil)
	require.NoError(t, err)

	resp, err := client.Do(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	counterVal := store.AggregatedCounterValue(vuhive.MetricHTTPReqs)
	assert.Equal(t, int64(1), counterVal)
}

func TestClient_Get_NonOKStatus_RecordsFailedRate(t *testing.T) {
	ts := newErrorServer(t)
	defer ts.Close()

	store, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	resp, err := client.Get(context.Background(), ts.URL+"/api/fail")
	require.NoError(t, err, "non-OK status is not a transport error")
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	// Verify failure rate was recorded (1.0 = 100% failures)
	failedRate := store.AggregatedRateValue(vuhive.MetricHTTPReqFailed)
	assert.Equal(t, 1.0, failedRate, "failure rate should be 1.0 for failed request")
}

func TestClient_Get_SuccessStatus_RecordsSuccessRate(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	store, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	_, err := client.Get(context.Background(), ts.URL+"/api/test")
	require.NoError(t, err)

	// Verify success rate (0.0 = 0% failures)
	failedRate := store.AggregatedRateValue(vuhive.MetricHTTPReqFailed)
	assert.Equal(t, 0.0, failedRate, "failure rate should be 0.0 for successful request")
}

func TestClient_Get_FailedRequest_RecordsFailedRate(t *testing.T) {
	store, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics, vuhivehttp.WithTimeout(100*time.Millisecond))

	// Request to a non-existent address
	_, err := client.Get(context.Background(), "http://127.0.0.1:1/nonexistent")
	assert.Error(t, err)

	// Metrics should still be recorded for the failed request
	counterVal := store.AggregatedCounterValue(vuhive.MetricHTTPReqs)
	assert.Equal(t, int64(1), counterVal, "counter should be 1 even for failed request")

	failedRate := store.AggregatedRateValue(vuhive.MetricHTTPReqFailed)
	assert.Equal(t, 1.0, failedRate, "failure rate should be 1.0 for transport failure")
}

func TestClient_Get_RespectsContextCancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics, vuhivehttp.WithTimeout(1*time.Second))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.Get(ctx, ts.URL+"/slow")
	assert.Error(t, err, "cancelled context should return an error")
}

func TestClient_Get_ResponseBodyParsing(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	resp, err := client.Get(context.Background(), ts.URL+"/api/test")
	require.NoError(t, err)

	// Verify Text()
	assert.Equal(t, `{"status":"ok"}`, resp.Text())

	// Verify JSON()
	var result map[string]string
	require.NoError(t, resp.JSON(&result))
	assert.Equal(t, "ok", result["status"])

	// Verify Headers
	assert.Equal(t, "application/json", resp.Headers.Get("Content-Type"))
}

func TestClient_MultipleRequests_AccumulateMetrics(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	store, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	for i := 0; i < 5; i++ {
		_, err := client.Get(context.Background(), ts.URL+"/api/test")
		require.NoError(t, err)
	}

	counterVal := store.AggregatedCounterValue(vuhive.MetricHTTPReqs)
	assert.Equal(t, int64(5), counterVal, "5 requests should produce 5 counter increments")
}

// --- Phase timing tests (opt-in) ---

func TestClient_Get_DetailedTiming_Disabled_NoPhaseMetrics(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	store, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics) // No WithDetailedTiming

	_, err := client.Get(context.Background(), ts.URL+"/api/test")
	require.NoError(t, err)

	// Phase metrics should NOT be recorded
	snap := store.MergedHistogramSnapshot(vuhive.MetricHTTPReqConnecting)
	assert.Equal(t, int64(0), snap.Count, "connecting metric should not be recorded without detailed timing")
}

func TestClient_Get_DetailedTiming_Enabled_RecordsPhaseMetrics(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	store, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics, vuhivehttp.WithDetailedTiming())

	resp, err := client.Get(context.Background(), ts.URL+"/api/test")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Core metrics should always be recorded
	counterVal := store.AggregatedCounterValue(vuhive.MetricHTTPReqs)
	assert.Equal(t, int64(1), counterVal)

	// Sending and receiving should be recorded for a successful request
	sendSnap := store.MergedHistogramSnapshot(vuhive.MetricHTTPReqSending)
	recvSnap := store.MergedHistogramSnapshot(vuhive.MetricHTTPReqReceiving)
	assert.Equal(t, int64(1), sendSnap.Count, "sending duration should be recorded with detailed timing")
	assert.Equal(t, int64(1), recvSnap.Count, "receiving duration should be recorded with detailed timing")
}

// --- Declarative HTTP Config Tests ---

type mockSetupContext struct {
	context.Context
	metrics vuhive.MetricsCollector
	httpCfg vuhive.HTTPConfig
}

func (m *mockSetupContext) Metrics() vuhive.MetricsCollector                          { return m.metrics }
func (m *mockSetupContext) Log() vuhive.Logger                                        { return nil }
func (m *mockSetupContext) Param(key string) string                                   { return "" }
func (m *mockSetupContext) ParamInt(key string, def int) int                          { return def }
func (m *mockSetupContext) ParamDuration(key string, def time.Duration) time.Duration { return def }
func (m *mockSetupContext) HTTPConfig() vuhive.HTTPConfig                             { return m.httpCfg }
func (m *mockSetupContext) HTTPClients() map[string]vuhive.HTTPConfig                 { return nil }

type mockVUContext struct {
	mockSetupContext
}

func (m *mockVUContext) VUID() int64                                                  { return 1 }
func (m *mockVUContext) Iteration() int64                                             { return 0 }
func (m *mockVUContext) ScenarioName() string                                         { return "mock" }
func (m *mockVUContext) GlobalState(key string) any                                   { return nil }
func (m *mockVUContext) Sleep(d ...time.Duration) error                               { return nil }
func (m *mockVUContext) Check(name string, fn vuhive.CheckFunc) bool                  { return true }
func (m *mockVUContext) CheckEqual(name string, actual, expected any) bool             { return true }
func (m *mockVUContext) CheckTrue(name string, condition bool, failureReason ...string) bool { return condition }
func (m *mockVUContext) CheckNoError(name string, err error) bool                      { return err == nil }
func (m *mockVUContext) Group(name string, fn func(ctx vuhive.VUContext) error) error { return fn(m) }

func TestNewClientFromConfig_FullDeclarativeConfig(t *testing.T) {
	var capturedAuth, capturedAccept string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	store, metrics := newTestStore(t)
	ctx := &mockSetupContext{
		Context: context.Background(),
		metrics: metrics,
		httpCfg: vuhive.HTTPConfig{
			BaseURL: ts.URL,
			Timeout: 5 * time.Second,
			Headers: map[string]string{
				"Authorization": "Bearer decl-token",
				"Accept":        "application/json",
			},
			TLS: vuhive.TLSConfig{
				InsecureSkipVerify: true,
			},
			Pool: vuhive.HTTPPoolConfig{
				MaxIdleConns:        50,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     60 * time.Second,
			},
			DetailedTiming: true,
			MetricPrefix:   "custom.http.",
		},
	}

	client := vuhivehttp.NewClientFromConfig(ctx)
	require.NotNil(t, client)
	assert.Equal(t, ts.URL, client.BaseURL())

	// Request with relative URL
	resp, err := client.Get(ctx, "/api/items")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Bearer decl-token", capturedAuth)
	assert.Equal(t, "application/json", capturedAccept)

	// Metrics with custom prefix and detailed timing
	counterVal := store.AggregatedCounterValue("custom.http.reqs")
	assert.Equal(t, int64(1), counterVal)
}

func TestNewClientFromConfig_ProgrammaticOverrides(t *testing.T) {
	var capturedAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	store, metrics := newTestStore(t)
	ctx := &mockSetupContext{
		Context: context.Background(),
		metrics: metrics,
		httpCfg: vuhive.HTTPConfig{
			BaseURL: "http://old-url.com",
			Headers: map[string]string{
				"Authorization": "Bearer default",
			},
			MetricPrefix: "decl.http.",
		},
	}

	// Programmatic options override declarative settings
	client := vuhivehttp.NewClientFromConfig(ctx,
		vuhivehttp.WithBaseURL(ts.URL),
		vuhivehttp.WithHeader("Authorization", "Bearer override"),
		vuhivehttp.WithCustomMetricPrefix("override.http."),
	)
	assert.Equal(t, ts.URL, client.BaseURL())

	_, err := client.Get(ctx, "/override-test")
	require.NoError(t, err)
	assert.Equal(t, "Bearer override", capturedAuth)

	counterVal := store.AggregatedCounterValue("override.http.reqs")
	assert.Equal(t, int64(1), counterVal)
}

func TestNewClientFromVUConfig(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	_, metrics := newTestStore(t)
	vuCtx := &mockVUContext{
		mockSetupContext: mockSetupContext{
			Context: context.Background(),
			metrics: metrics,
			httpCfg: vuhive.HTTPConfig{
				BaseURL: ts.URL,
				Timeout: 2 * time.Second,
			},
		},
	}

	client := vuhivehttp.NewClientFromVUConfig(vuCtx)
	require.NotNil(t, client)
	assert.Equal(t, ts.URL, client.BaseURL())

	resp, err := client.Get(vuCtx, "/test-vu")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestClient_BaseURL_RelativeAndAbsoluteURLs(t *testing.T) {
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/relative/path", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts1.Close()

	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/absolute/path", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts2.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics, vuhivehttp.WithBaseURL(ts1.URL))
	assert.Equal(t, ts1.URL, client.BaseURL())

	// Relative URL should hit ts1
	resp1, err := client.Get(context.Background(), "/relative/path")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	// Absolute URL should hit ts2
	resp2, err := client.Get(context.Background(), ts2.URL+"/absolute/path")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
}

func TestDefault_LazySharedSingleton(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	store, metrics := newTestStore(t)
	vuCtx := &mockVUContext{
		mockSetupContext: mockSetupContext{
			Context: context.Background(),
			metrics: metrics,
			httpCfg: vuhive.HTTPConfig{
				BaseURL: ts.URL,
				Timeout: 3 * time.Second,
				Headers: map[string]string{
					"X-Test": "default-val",
				},
			},
		},
	}

	// 1. Initial call lazily creates singleton
	c1 := vuhivehttp.Default(vuCtx)
	require.NotNil(t, c1)
	assert.Equal(t, ts.URL, c1.BaseURL())

	// 2. Subsequent call returns exact same instance
	c2 := vuhivehttp.Default(vuCtx)
	assert.Same(t, c1, c2, "Default(ctx) must return identical shared singleton for the same scenario")

	// 3. Execution works and records metrics
	resp, err := c1.Get(vuCtx, "/api/test")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int64(1), store.AggregatedCounterValue(vuhive.MetricHTTPReqs))

	// 4. Concurrent calls from multiple goroutines return the same singleton safely
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := vuhivehttp.Default(vuCtx)
			assert.Same(t, c1, client)
		}()
	}
	wg.Wait()
}

func TestClient_Do_NilContextFallback(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	_, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/nil-ctx-test", nil)
	require.NoError(t, err)

	//nolint:staticcheck // SA1012: intentionally testing nil context fallback
	resp, err := client.Do(nil, req)
	require.NoError(t, err, "client.Do(nil, req) should not panic or fail")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestClient_StandardClient_RestRequests(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Echo-Header", r.Header.Get("X-Custom"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"standard-ok"}`))
	}))
	defer ts.Close()

	store, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)
	stdClient := client.StandardClient()
	require.NotNil(t, stdClient)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/std-rest", nil)
	require.NoError(t, err)
	req.Header.Set("X-Custom", "val-123")

	resp, err := stdClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "val-123", resp.Header.Get("X-Echo-Header"))

	counterVal := store.AggregatedCounterValue(vuhive.MetricHTTPReqs)
	assert.Equal(t, int64(1), counterVal, "StandardClient REST request should record standard metrics")
}

func TestClient_StandardClient_SSERequests(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		_, _ = w.Write([]byte("data: event-1\n\n"))
		flusher.Flush()
		_, _ = w.Write([]byte("data: event-2\n\n"))
		flusher.Flush()
	}))
	defer ts.Close()

	store, metrics := newTestStore(t)
	client := vuhivehttp.NewClientWithCollector(metrics)
	stdClient := client.StandardClient()
	require.NotNil(t, stdClient)

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet, ts.URL+"/std-sse", nil,
	)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := stdClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Third-party SDKs use bufio.Scanner to read SSE — verify the raw
	// streaming body is passed through unmodified.
	scanner := bufio.NewScanner(resp.Body)
	var dataLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	require.NoError(t, scanner.Err())
	assert.Equal(t, []string{"event-1", "event-2"}, dataLines)

	assert.Equal(t, int64(1), store.AggregatedCounterValue(vuhive.MetricHTTPSSEConnectionsTotal))
}

