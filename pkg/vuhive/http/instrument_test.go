package http_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/morphy76/vuhive/pkg/vuhive"
	vuhivehttp "github.com/morphy76/vuhive/pkg/vuhive/http"
)

// testVUContext mocks a VU context carrying a MetricsCollector.
type testVUContext struct {
	context.Context
	metrics vuhive.MetricsCollector
}

func (c *testVUContext) Log() vuhive.Logger {
	return nil
}

func (c *testVUContext) Metrics() vuhive.MetricsCollector {
	return c.metrics
}

func (c *testVUContext) Value(key any) any {
	if key == "vuhive.metrics" {
		return c.metrics
	}
	return c.Context.Value(key)
}

func TestInstrument_StandardRequest_RecordsMetrics(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	store, metrics := newTestStore(t)
	vuCtx := &testVUContext{
		Context: context.Background(),
		metrics: metrics,
	}

	baseClient := &http.Client{Timeout: 5 * time.Second}
	instrumentedClient := vuhivehttp.Instrument(baseClient)
	require.NotNil(t, instrumentedClient)

	req, err := http.NewRequestWithContext(vuCtx, http.MethodGet, ts.URL+"/checkout", nil)
	require.NoError(t, err)

	resp, err := instrumentedClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"status":"ok"}`, string(body))
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify metrics recorded
	assert.Equal(t, int64(1), store.AggregatedCounterValue("vuhive.http.reqs"))
	snap := store.MergedHistogramSnapshot("vuhive.http.req_duration")
	assert.Equal(t, int64(1), snap.Count)
	assert.True(t, snap.Mean > 0)
	assert.Equal(t, float64(0), store.AggregatedRateValue("vuhive.http.req_failed"))
}

func TestInstrument_ErrorResponse_RecordsFailedRateAndCounter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server error"}`))
	}))
	defer ts.Close()

	store, metrics := newTestStore(t)
	vuCtx := &testVUContext{
		Context: context.Background(),
		metrics: metrics,
	}

	instrumentedClient := vuhivehttp.Instrument(&http.Client{})
	req, err := http.NewRequestWithContext(vuCtx, http.MethodPost, ts.URL+"/items", nil)
	require.NoError(t, err)

	resp, err := instrumentedClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	assert.Equal(t, int64(1), store.AggregatedCounterValue("vuhive.http.reqs"))
	assert.Equal(t, float64(1), store.AggregatedRateValue("vuhive.http.req_failed"))
}

func TestInstrument_NetworkError_RecordsFailedMetrics(t *testing.T) {
	store, metrics := newTestStore(t)
	vuCtx := &testVUContext{
		Context: context.Background(),
		metrics: metrics,
	}

	instrumentedClient := vuhivehttp.Instrument(&http.Client{})
	// Use an invalid port to force connection failure
	req, err := http.NewRequestWithContext(vuCtx, http.MethodGet, "http://127.0.0.1:1/unreachable", nil)
	require.NoError(t, err)

	resp, err := instrumentedClient.Do(req)
	assert.Error(t, err)
	assert.Nil(t, resp)

	assert.Equal(t, int64(1), store.AggregatedCounterValue("vuhive.http.reqs"))
	assert.Equal(t, float64(1), store.AggregatedRateValue("vuhive.http.req_failed"))
}

func TestInstrument_CustomMetricPrefixAndTags(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	store, metrics := newTestStore(t)
	vuCtx := &testVUContext{
		Context: context.Background(),
		metrics: metrics,
	}

	customPrefix := "custom.api."
	staticTags := vuhive.Tags{"env": "staging", "service": "billing"}

	client := vuhivehttp.Instrument(
		&http.Client{},
		vuhivehttp.WithMetricPrefix(customPrefix),
		vuhivehttp.WithTags(staticTags),
	)

	req, err := http.NewRequestWithContext(vuCtx, http.MethodGet, ts.URL+"/pay", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, int64(1), store.AggregatedCounterValue(customPrefix+"reqs"))
	snap := store.MergedHistogramSnapshot(customPrefix + "req_duration")
	assert.Equal(t, int64(1), snap.Count)
}

func TestInstrument_DetailedTiming_RecordsPhaseHistograms(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	store, metrics := newTestStore(t)
	vuCtx := &testVUContext{
		Context: context.Background(),
		metrics: metrics,
	}

	client := vuhivehttp.Instrument(
		&http.Client{
			Transport: &http.Transport{
				DisableKeepAlives: true, // Force TCP connect on every request to trigger connect trace
			},
		},
		vuhivehttp.WithInstrumentDetailedTiming(true),
	)

	req, err := http.NewRequestWithContext(vuCtx, http.MethodGet, ts.URL+"/timing", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	connectSnap := store.MergedHistogramSnapshot("vuhive.http.req_connecting")
	assert.True(t, connectSnap.Count >= 1, "vuhive.http.req_connecting should have observations")
}

func TestInstrument_NilClient_UsesDefaultClient(t *testing.T) {
	client := vuhivehttp.Instrument(nil)
	require.NotNil(t, client)
	assert.NotNil(t, client.Transport)
}

func TestInstrumentTransport_NilBase_UsesDefaultTransport(t *testing.T) {
	transport := vuhivehttp.InstrumentTransport(nil)
	require.NotNil(t, transport)
}

func TestInstrument_NonVUContext_ExecutesWithoutPanics(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := vuhivehttp.Instrument(&http.Client{})

	// Standard context.Background() without any VUContext / MetricsCollector
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/no-metrics", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestInstrument_WrappedContext_Timeout_ExtractsMetrics(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	store, metrics := newTestStore(t)
	vuCtx := &testVUContext{
		Context: context.Background(),
		metrics: metrics,
	}

	// Wrap VUContext in standard library context.WithTimeout
	timeoutCtx, cancel := context.WithTimeout(vuCtx, 2*time.Second)
	defer cancel()

	client := vuhivehttp.Instrument(&http.Client{})

	req, err := http.NewRequestWithContext(timeoutCtx, http.MethodGet, ts.URL+"/wrapped", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, int64(1), store.AggregatedCounterValue("vuhive.http.reqs"))
}

func TestInstrument_NilRequest_ReturnsError(t *testing.T) {
	transport := vuhivehttp.InstrumentTransport(nil)
	resp, err := transport.RoundTrip(nil)
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestInstrumentTransport_DirectRoundTrip(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	store, metrics := newTestStore(t)
	vuCtx := &testVUContext{
		Context: context.Background(),
		metrics: metrics,
	}

	transport := vuhivehttp.InstrumentTransport(http.DefaultTransport)
	req, err := http.NewRequestWithContext(vuCtx, http.MethodGet, ts.URL+"/direct", nil)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, int64(1), store.AggregatedCounterValue("vuhive.http.reqs"))
}

