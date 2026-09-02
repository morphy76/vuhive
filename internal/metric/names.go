package metric

// Built-in telemetry metric name constants.
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

	// MetricVURestartsTotal tracks the total number of supervisor worker restart/retry attempts.
	MetricVURestartsTotal = "vuhive.vu.restarts_total"

	// MetricVUStalledIterations tracks the total number of stalled VU iterations detected by the watchdog.
	MetricVUStalledIterations = "vuhive.vu.stalled_iterations"

	// MetricPacingDroppedIterations tracks the number of arrival-rate iterations dropped due to pool saturation.
	MetricPacingDroppedIterations = "vuhive.pacing.dropped_iterations"

	// MetricChecksPassed tracks the count of passed inline checks.
	MetricChecksPassed = "vuhive.checks.passed"

	// MetricChecksFailed tracks the count of failed inline checks.
	MetricChecksFailed = "vuhive.checks.failed"

	// MetricGroupPrefix is the reserved prefix for all transaction group duration metrics.
	MetricGroupPrefix = "vuhive.group."

	// MetricGroupSuffix is the reserved suffix for all transaction group duration metrics.
	MetricGroupSuffix = ".duration"
)

// GroupMetricName formats the full metric name for a given transaction group path.
func GroupMetricName(groupPath string) string {
	return MetricGroupPrefix + groupPath + MetricGroupSuffix
}

