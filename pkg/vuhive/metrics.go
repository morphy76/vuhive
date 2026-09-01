package vuhive

import "time"

// Tags is an optional set of key-value labels attached to a metric observation.
type Tags map[string]string

// Counter is a monotonically increasing counter.
type Counter interface {
	// Inc increments the counter by 1.
	Inc()

	// Add increments the counter by the specified delta.
	Add(delta int64)
}

// Gauge is an instantaneous value handle.
type Gauge interface {
	// Set sets the gauge to the specified float64 value.
	Set(value float64)

	// Add adjusts the gauge value by adding delta.
	Add(delta float64)
}

// Duration records latency samples into an HDR histogram.
type Duration interface {
	// Observe records a latency duration observation.
	Observe(d time.Duration)
}

// Rate tracks a ratio of numerator to denominator.
type Rate interface {
	// Add increments the numerator and denominator accumulators.
	Add(numerator, denominator int64)
}

// MetricsCollector is available inside ScenarioContext.
// All returned metric handles are safe for concurrent use from multiple VU goroutines.
type MetricsCollector interface {
	// Counter returns a thread-safe handle to a named counter metric with the given tags.
	Counter(name string, tags Tags) Counter

	// Gauge returns a thread-safe handle to a named gauge metric with the given tags.
	Gauge(name string, tags Tags) Gauge

	// Duration returns a thread-safe handle to a named duration histogram metric with the given tags.
	Duration(name string, tags Tags) Duration

	// Rate returns a thread-safe handle to a named rate metric with the given tags.
	Rate(name string, tags Tags) Rate
}

// Built-in framework metric names.
const (
	// MetricPrefix is the prefix reserved for all built-in framework telemetry metrics.
	MetricPrefix = "vuhive."

	// MetricVUActive tracks the number of currently active VU goroutines.
	MetricVUActive = "vuhive.vu.active"

	// MetricVUPanics tracks the total number of recovered panics in RunVU.
	MetricVUPanics = "vuhive.vu.panics"

	// MetricIterationsTotal tracks the total number of completed VU iterations.
	MetricIterationsTotal = "vuhive.vu.iterations_total"

	// MetricIterationsFailed tracks the total number of failed VU iterations (errors or panics).
	MetricIterationsFailed = "vuhive.vu.iterations_failed"

	// MetricIterationsTimeout tracks the total number of VU iterations that exceeded vu_timeout.
	MetricIterationsTimeout = "vuhive.vu.iterations_timeout"

	// MetricIterationDuration records latency of VU iterations.
	MetricIterationDuration = "vuhive.vu.iteration_duration"

	// MetricVUPretestErrors tracks the number of PreTest hook errors.
	MetricVUPretestErrors = "vuhive.vu.pretest_errors"

	// MetricPacingDroppedIterations tracks the number of arrival-rate iterations dropped due to pool saturation.
	MetricPacingDroppedIterations = "vuhive.pacing.dropped_iterations"

	// MetricChecksPassed tracks the count of passed inline checks.
	MetricChecksPassed = "vuhive.checks.passed"

	// MetricChecksFailed tracks the count of failed inline checks.
	MetricChecksFailed = "vuhive.checks.failed"

	// MetricHTTPPrefix is the prefix for all built-in HTTP module metrics.
	MetricHTTPPrefix = "vuhive.http."

	// MetricHTTPReqDuration records total HTTP request latency (Duration histogram).
	MetricHTTPReqDuration = "vuhive.http.req_duration"

	// MetricHTTPReqFailed tracks the HTTP request failure rate (Rate).
	MetricHTTPReqFailed = "vuhive.http.req_failed"

	// MetricHTTPReqs tracks the total HTTP request count (Counter).
	MetricHTTPReqs = "vuhive.http.reqs"

	// MetricHTTPReqConnecting records TCP connection establishment time (Duration, opt-in).
	MetricHTTPReqConnecting = "vuhive.http.req_connecting"

	// MetricHTTPReqTLSHandshaking records TLS handshake time (Duration, opt-in).
	MetricHTTPReqTLSHandshaking = "vuhive.http.req_tls_handshaking"

	// MetricHTTPReqSending records request write time (Duration, opt-in).
	MetricHTTPReqSending = "vuhive.http.req_sending"

	// MetricHTTPReqReceiving records response read time (Duration, opt-in).
	MetricHTTPReqReceiving = "vuhive.http.req_receiving"

	// MetricHTTPSSEPrefix is the prefix for all built-in HTTP SSE metrics.
	MetricHTTPSSEPrefix = "vuhive.http.sse."

	// MetricHTTPSSEConnectionsTotal tracks the total number of SSE connection attempts (Counter).
	MetricHTTPSSEConnectionsTotal = "vuhive.http.sse.connections_total"

	// MetricHTTPSSEConnectDuration records latency to establish SSE connection and receive headers (Duration histogram).
	MetricHTTPSSEConnectDuration = "vuhive.http.sse.connect_duration"

	// MetricHTTPSSEEventsTotal tracks the total number of received SSE events (Counter).
	MetricHTTPSSEEventsTotal = "vuhive.http.sse.events_total"

	// MetricHTTPSSEEventLatency records inter-arrival latency between successive SSE events (Duration histogram).
	MetricHTTPSSEEventLatency = "vuhive.http.sse.event_latency"

	// MetricHTTPSSEStreamDuration records the total active duration of an SSE stream (Duration histogram).
	MetricHTTPSSEStreamDuration = "vuhive.http.sse.stream_duration"

	// MetricHTTPSSEErrorsTotal tracks SSE connection/stream errors (Counter).
	MetricHTTPSSEErrorsTotal = "vuhive.http.sse.errors_total"

	// MetricKafkaPrefix is the prefix for all built-in Kafka module metrics.
	MetricKafkaPrefix = "vuhive.kafka."

	// MetricKafkaPubDuration records Kafka publish round-trip latency (Duration histogram).
	MetricKafkaPubDuration = "vuhive.kafka.pub_duration"

	// MetricKafkaPubTotal tracks total Kafka messages published (Counter).
	MetricKafkaPubTotal = "vuhive.kafka.pub_total"

	// MetricKafkaPubBytes tracks total Kafka payload bytes published (Counter).
	MetricKafkaPubBytes = "vuhive.kafka.pub_bytes"

	// MetricKafkaPubFailed tracks Kafka publish failure rate (Rate).
	MetricKafkaPubFailed = "vuhive.kafka.pub_failed"

	// MetricKafkaSubDuration records Kafka consumer fetch/wait duration (Duration histogram).
	MetricKafkaSubDuration = "vuhive.kafka.sub_duration"

	// MetricKafkaSubTotal tracks total Kafka messages consumed (Counter).
	MetricKafkaSubTotal = "vuhive.kafka.sub_total"

	// MetricKafkaSubBytes tracks total Kafka payload bytes consumed (Counter).
	MetricKafkaSubBytes = "vuhive.kafka.sub_bytes"

	// MetricKafkaSubFailed tracks Kafka consumer failure rate (Rate).
	MetricKafkaSubFailed = "vuhive.kafka.sub_failed"

	// MetricNATSPrefix is the prefix for all built-in NATS module metrics.
	MetricNATSPrefix = "vuhive.nats."

	// MetricNATSPubDuration records NATS publish latency (Duration histogram).
	MetricNATSPubDuration = "vuhive.nats.pub_duration"

	// MetricNATSPubTotal tracks total NATS messages published (Counter).
	MetricNATSPubTotal = "vuhive.nats.pub_total"

	// MetricNATSPubBytes tracks total NATS payload bytes published (Counter).
	MetricNATSPubBytes = "vuhive.nats.pub_bytes"

	// MetricNATSPubFailed tracks NATS publish failure rate (Rate).
	MetricNATSPubFailed = "vuhive.nats.pub_failed"

	// MetricNATSReqDuration records NATS request-reply round-trip latency (Duration histogram).
	MetricNATSReqDuration = "vuhive.nats.req_duration"

	// MetricNATSSubReceivedTotal tracks total NATS messages received (Counter).
	MetricNATSSubReceivedTotal = "vuhive.nats.sub_received_total"

	// MetricNATSSubBytes tracks total NATS payload bytes read (Counter).
	MetricNATSSubBytes = "vuhive.nats.sub_bytes"

	// MetricNATSSubFailed tracks NATS receive failure rate (Rate).
	MetricNATSSubFailed = "vuhive.nats.sub_failed"

	// MetricGroupPrefix is the prefix for all built-in transaction group duration metrics.
	MetricGroupPrefix = "vuhive.group."

	// MetricGroupSuffix is the suffix for all built-in transaction group duration metrics.
	MetricGroupSuffix = ".duration"
)

// GroupMetricName formats the full metric name for a given transaction group path.
func GroupMetricName(groupPath string) string {
	return MetricGroupPrefix + groupPath + MetricGroupSuffix
}

