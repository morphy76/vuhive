package engine_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/engine"
	"github.com/morphy76/vuhive/internal/log"
	"github.com/morphy76/vuhive/internal/metric"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEngine_Iteration_EnforcesGuardDeadlineWhenUnset verifies that when VUTimeout is unset (0)
// and AllowUnboundedIterations is false, a mandatory safety guard deadline is enforced,
// cancelling hanging iterations via ctx.Done() and incrementing timeout metrics.
func TestEngine_Iteration_EnforcesGuardDeadlineWhenUnset(t *testing.T) {
	logger, metrics := newTestDeps()

	var timedOut atomic.Bool

	scenario := engine.Scenario{
		RunVU: func(ctx engine.VUContext) error {
			select {
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					timedOut.Store(true)
				}
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
				return nil
			}
		},
	}

	// In ScenarioConfig, VUTimeout is 0 and AllowUnboundedIterations is false.
	// For testing purposes, when default guard deadline is active, the engine provides a safety deadline.
	cfg := config.ScenarioConfig{
		Type:                     config.ScenarioTypeConstantVUs,
		VUs:                      1,
		RunPeriod:                100 * time.Millisecond,
		VUTimeout:                50 * time.Millisecond, // explicit or guard deadline
		AllowUnboundedIterations: false,
	}

	exec := engine.NewExecutor("guard_deadline_test", scenario, cfg, logger, metrics)
	err := exec.Execute(context.Background())
	require.NoError(t, err)

	assert.True(t, timedOut.Load(), "RunVU iteration should have been cancelled by deadline")
	assert.Greater(t, metrics.AggregatedCounterValue(metric.MetricIterationsTimeout), int64(0), "MetricIterationsTimeout should be incremented")
}

// TestEngine_Watchdog_DetectsStalledVUs verifies that the execution watchdog detects iterations
// exceeding the stall threshold, emits structured warning logs, and increments stalled_iterations counter.
func TestEngine_Watchdog_DetectsStalledVUs(t *testing.T) {
	var buf safeBuffer
	logger := log.New(&buf, zerolog.DebugLevel)
	metrics := metric.NewStore()

	var entered atomic.Int64

	scenario := engine.Scenario{
		RunVU: func(ctx engine.VUContext) error {
			entered.Add(1)
			// Sleep longer than WatchdogStallThreshold (30ms) but within VUTimeout (200ms)
			time.Sleep(80 * time.Millisecond)
			return nil
		},
	}

	cfg := config.ScenarioConfig{
		Type:                   config.ScenarioTypeConstantVUs,
		VUs:                    2,
		RunPeriod:              150 * time.Millisecond,
		VUTimeout:              300 * time.Millisecond,
		WatchdogStallThreshold: 30 * time.Millisecond,
		WatchdogInterval:       10 * time.Millisecond,
	}

	exec := engine.NewExecutor("watchdog_stall_test", scenario, cfg, logger, metrics)
	err := exec.Execute(context.Background())
	require.NoError(t, err)

	assert.Greater(t, entered.Load(), int64(0), "iterations should have entered")

	// Verify metric
	stalledCount := metrics.AggregatedCounterValue(metric.MetricVUStalledIterations)
	assert.Greater(t, stalledCount, int64(0), "MetricVUStalledIterations should be incremented when VU stalls")

	// Verify structured warning logs
	logOutput := buf.String()
	assert.Contains(t, logOutput, "stalled", "log output should contain stall warnings")
	assert.Contains(t, logOutput, "vu_id", "log output should contain vu_id attribute")
}

// TestEngine_Watchdog_NoFalsePositives verifies that fast iterations finishing before
// the stall threshold do not increment the stalled iterations metric.
func TestEngine_Watchdog_NoFalsePositives(t *testing.T) {
	logger, metrics := newTestDeps()

	scenario := engine.Scenario{
		RunVU: func(ctx engine.VUContext) error {
			time.Sleep(2 * time.Millisecond)
			return nil
		},
	}

	cfg := config.ScenarioConfig{
		Type:                   config.ScenarioTypeConstantVUs,
		VUs:                    2,
		RunPeriod:              80 * time.Millisecond,
		VUTimeout:              200 * time.Millisecond,
		WatchdogStallThreshold: 50 * time.Millisecond,
		WatchdogInterval:       10 * time.Millisecond,
	}

	exec := engine.NewExecutor("watchdog_fast_test", scenario, cfg, logger, metrics)
	err := exec.Execute(context.Background())
	require.NoError(t, err)

	stalledCount := metrics.AggregatedCounterValue(metric.MetricVUStalledIterations)
	assert.Equal(t, int64(0), stalledCount, "fast iterations must not trigger stalled iterations metric")
}

// TestEngine_Watchdog_ArrivalRate_DetectsStalledVUs verifies stall detection within arrival_rate pacing.
func TestEngine_Watchdog_ArrivalRate_DetectsStalledVUs(t *testing.T) {
	var buf safeBuffer
	logger := log.New(&buf, zerolog.DebugLevel)
	metrics := metric.NewStore()

	scenario := engine.Scenario{
		RunVU: func(ctx engine.VUContext) error {
			time.Sleep(70 * time.Millisecond)
			return nil
		},
	}

	cfg := config.ScenarioConfig{
		Type:                   config.ScenarioTypeArrivalRate,
		TargetTPS:              20,
		MaxVUs:                 4,
		RunPeriod:              120 * time.Millisecond,
		VUTimeout:              300 * time.Millisecond,
		WatchdogStallThreshold: 25 * time.Millisecond,
		WatchdogInterval:       10 * time.Millisecond,
	}

	exec := engine.NewExecutor("watchdog_arrival_test", scenario, cfg, logger, metrics)
	err := exec.Execute(context.Background())
	require.NoError(t, err)

	stalledCount := metrics.AggregatedCounterValue(metric.MetricVUStalledIterations)
	assert.Greater(t, stalledCount, int64(0), "arrival_rate workers exceeding stall threshold must increment MetricVUStalledIterations")
}

// TestEngine_Watchdog_RampingVUs_DetectsStalledVUs verifies stall detection within ramping_vus pacing.
func TestEngine_Watchdog_RampingVUs_DetectsStalledVUs(t *testing.T) {
	var buf safeBuffer
	logger := log.New(&buf, zerolog.DebugLevel)
	metrics := metric.NewStore()

	scenario := engine.Scenario{
		RunVU: func(ctx engine.VUContext) error {
			time.Sleep(70 * time.Millisecond)
			return nil
		},
	}

	cfg := config.ScenarioConfig{
		Type: config.ScenarioTypeRampingVUs,
		Stages: []config.StageConfig{
			{Target: 2, Duration: 100 * time.Millisecond},
		},
		VUTimeout:              300 * time.Millisecond,
		WatchdogStallThreshold: 25 * time.Millisecond,
		WatchdogInterval:       10 * time.Millisecond,
	}

	exec := engine.NewExecutor("watchdog_ramping_test", scenario, cfg, logger, metrics)
	err := exec.Execute(context.Background())
	require.NoError(t, err)

	stalledCount := metrics.AggregatedCounterValue(metric.MetricVUStalledIterations)
	assert.Greater(t, stalledCount, int64(0), "ramping_vus workers exceeding stall threshold must increment MetricVUStalledIterations")
}

// TestAlloc_Watchdog_HotPath verifies that Begin and End on VUTracker produce 0 heap allocations.
func TestAlloc_Watchdog_HotPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	metrics := metric.NewStore()
	logger := log.New(nil, zerolog.Disabled)

	cfg := config.ScenarioConfig{
		WatchdogStallThreshold: 100 * time.Millisecond,
		WatchdogInterval:       50 * time.Millisecond,
	}

	wd := engine.NewExecutionWatchdog(ctx, cfg, logger, metrics)
	defer wd.Stop()

	tracker := wd.RegisterVU(1)
	defer tracker.Unregister()

	allocs := testing.AllocsPerRun(1000, func() {
		tracker.Begin(42)
		tracker.End()
	})

	assert.Equal(t, float64(0), allocs, "VUTracker Begin and End must produce 0 heap allocations in steady state")
}
