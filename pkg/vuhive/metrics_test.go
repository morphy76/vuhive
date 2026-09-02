package vuhive_test

import (
	"testing"

	"github.com/morphy76/vuhive/pkg/vuhive"
	"github.com/stretchr/testify/assert"
)

func TestPublicMetricConstants(t *testing.T) {
	assert.Equal(t, "vuhive.", vuhive.MetricPrefix)
	assert.Equal(t, "vuhive.vu.active", vuhive.MetricVUActive)
	assert.Equal(t, "vuhive.vu.panics", vuhive.MetricVUPanics)
	assert.Equal(t, "vuhive.vu.iterations_total", vuhive.MetricIterationsTotal)
	assert.Equal(t, "vuhive.vu.iterations_failed", vuhive.MetricIterationsFailed)
	assert.Equal(t, "vuhive.vu.iterations_timeout", vuhive.MetricIterationsTimeout)
	assert.Equal(t, "vuhive.vu.iteration_duration", vuhive.MetricIterationDuration)
	assert.Equal(t, "vuhive.vu.pretest_errors", vuhive.MetricVUPretestErrors)
	assert.Equal(t, "vuhive.vu.restarts_total", vuhive.MetricVURestartsTotal)
	assert.Equal(t, "vuhive.vu.stalled_iterations", vuhive.MetricVUStalledIterations)
	assert.Equal(t, "vuhive.pacing.dropped_iterations", vuhive.MetricPacingDroppedIterations)
	assert.Equal(t, "vuhive.checks.passed", vuhive.MetricChecksPassed)
	assert.Equal(t, "vuhive.checks.failed", vuhive.MetricChecksFailed)
}
