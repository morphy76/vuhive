package http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"time"

	"github.com/morphy76/vuhive/pkg/vuhive"
)

// InstrumentOption configures behavior of the instrumented HTTP transport.
type InstrumentOption func(*instrumentedTransport)

// WithMetricPrefix sets the metric name prefix (defaults to "vuhive.http.").
func WithMetricPrefix(prefix string) InstrumentOption {
	return func(t *instrumentedTransport) {
		t.metricPrefix = prefix
	}
}

// WithInstrumentDetailedTiming enables httptrace per-phase metric recording (DNS, connect, TLS, sending).
func WithInstrumentDetailedTiming(enabled ...bool) InstrumentOption {
	val := true
	if len(enabled) > 0 {
		val = enabled[0]
	}
	return func(t *instrumentedTransport) {
		t.detailedTiming = val
	}
}

// WithTags adds default static tags to all metric observations from this client.
func WithTags(tags vuhive.Tags) InstrumentOption {
	return func(t *instrumentedTransport) {
		if len(tags) > 0 {
			if t.tags == nil {
				t.tags = make(vuhive.Tags, len(tags))
			}
			for k, v := range tags {
				t.tags[k] = v
			}
		}
	}
}

type instrumentedTransport struct {
	base           http.RoundTripper
	metricPrefix   string
	detailedTiming bool
	tags           vuhive.Tags
}

// InstrumentTransport wraps an existing http.RoundTripper with vuhive metrics collection.
// Metrics (duration, total requests, failure rate) are extracted using the ObservabilityProvider
// or MetricsCollector attached to the request context (VUContext).
func InstrumentTransport(base http.RoundTripper, opts ...InstrumentOption) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	t := &instrumentedTransport{
		base:         base,
		metricPrefix: defaultMetricPrefix,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Instrument returns a shallow copy of client whose Transport is wrapped with vuhive metrics collection.
// If client is nil, a shallow copy of http.DefaultClient is used as the base.
func Instrument(client *http.Client, opts ...InstrumentOption) *http.Client {
	var c http.Client
	if client != nil {
		c = *client
	} else {
		c = *http.DefaultClient
	}

	c.Transport = InstrumentTransport(c.Transport, opts...)
	return &c
}

func (t *instrumentedTransport) buildTags(method, url, status string) vuhive.Tags {
	tags := make(vuhive.Tags, len(t.tags)+3)
	for k, v := range t.tags {
		tags[k] = v
	}
	tags["method"] = method
	tags["url"] = url
	tags["status"] = status
	return tags
}

func extractMetrics(ctx context.Context) vuhive.MetricsCollector {
	if ctx == nil {
		return nil
	}
	if op, ok := ctx.(interface{ Metrics() vuhive.MetricsCollector }); ok {
		if m := op.Metrics(); m != nil {
			return m
		}
	}
	if mc, ok := ctx.(vuhive.MetricsCollector); ok {
		return mc
	}
	if val := ctx.Value("vuhive.metrics"); val != nil {
		if mc, ok := val.(vuhive.MetricsCollector); ok {
			return mc
		}
	}
	return nil
}

func (t *instrumentedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("vuhive/http: request must not be nil")
	}

	metrics := extractMetrics(req.Context())
	if metrics == nil {
		return t.base.RoundTrip(req)
	}

	method := req.Method
	metricURL := sanitizeURL(req.URL)

	var timings traceTimings
	execReq := req
	if t.detailedTiming {
		traceCtx := httptrace.WithClientTrace(req.Context(), newClientTrace(&timings))
		execReq = req.WithContext(traceCtx)
	}

	start := time.Now()
	resp, err := t.base.RoundTrip(execReq)
	totalDuration := time.Since(start)

	if err != nil {
		tags := t.buildTags(method, metricURL, "0")
		metrics.Duration(t.metricPrefix+MetricSuffixReqDuration, tags).Observe(totalDuration)
		metrics.Counter(t.metricPrefix+MetricSuffixReqs, tags).Inc()
		metrics.Rate(t.metricPrefix+MetricSuffixReqFailed, tags).Add(1, 1)
		return nil, err
	}

	statusCode := resp.StatusCode
	tags := t.buildTags(method, metricURL, strconv.Itoa(statusCode))
	failed := statusCode < 200 || statusCode >= 400

	metrics.Duration(t.metricPrefix+MetricSuffixReqDuration, tags).Observe(totalDuration)
	metrics.Counter(t.metricPrefix+MetricSuffixReqs, tags).Inc()
	if failed {
		metrics.Rate(t.metricPrefix+MetricSuffixReqFailed, tags).Add(1, 1)
	} else {
		metrics.Rate(t.metricPrefix+MetricSuffixReqFailed, tags).Add(0, 1)
	}

	if t.detailedTiming {
		if timings.connectDuration > 0 {
			metrics.Duration(t.metricPrefix+MetricSuffixReqConnecting, tags).Observe(timings.connectDuration)
		}
		if timings.tlsDuration > 0 {
			metrics.Duration(t.metricPrefix+MetricSuffixReqTLSHandshaking, tags).Observe(timings.tlsDuration)
		}
		if !timings.wroteHeaders.IsZero() && !timings.gotFirstByte.IsZero() {
			sendingDuration := timings.gotFirstByte.Sub(timings.wroteHeaders)
			if sendingDuration > 0 {
				metrics.Duration(t.metricPrefix+MetricSuffixReqSending, tags).Observe(sendingDuration)
			}
		}
	}

	return resp, nil
}

// Compile-time static interface assertion.
var _ http.RoundTripper = (*instrumentedTransport)(nil)
