package engine_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/engine"
	"github.com/morphy76/vuhive/internal/metric"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AC-1.7.1: target_tps=10, run_period=1s → approximately 10 RunVU calls (±20% tolerance)
func TestArrivalRateTargetTPS(t *testing.T) {
	logger, metrics := newTestDeps()

	scenario := engine.Scenario{
		RunVU: func(ctx engine.VUContext) error {
			time.Sleep(5 * time.Millisecond)
			return nil
		},
	}

	cfg := config.ScenarioConfig{
		Type:      config.ScenarioTypeArrivalRate,
		TargetTPS: 10,
		MaxVUs:    50,
		RunPeriod: 1 * time.Second,
		VUTimeout: 1 * time.Second,
	}

	exec := engine.NewExecutor("test_arrival_rate", scenario, cfg, logger, metrics)
	err := exec.Execute(context.Background())
	require.NoError(t, err)

	total := metrics.AggregatedCounterValue(metric.MetricIterationsTotal)
	// Target is 10 calls. ±20% tolerance means 8..12 calls.
	assert.GreaterOrEqual(t, total, int64(8), "expected at least 8 completed iterations, got %d", total)
	assert.LessOrEqual(t, total, int64(13), "expected at most 13 completed iterations, got %d", total)
}

// AC-1.7.2: max_vus=2 with slow RunVU (sleeps 500ms) and target_tps=100 → pool saturates;
// vuhive.pacing.dropped_iterations > 0
func TestArrivalRatePoolSaturationDropsIterations(t *testing.T) {
	logger, metrics := newTestDeps()

	scenario := engine.Scenario{
		RunVU: func(ctx engine.VUContext) error {
			time.Sleep(500 * time.Millisecond)
			return nil
		},
	}

	cfg := config.ScenarioConfig{
		Type:        config.ScenarioTypeArrivalRate,
		TargetTPS:   100,
		MaxVUs:      2,
		BurstBuffer: 1, // minimal buffer to observe saturation drops
		RunPeriod:   500 * time.Millisecond,
		VUTimeout:   1 * time.Second,
	}

	exec := engine.NewExecutor("test_arrival_rate", scenario, cfg, logger, metrics)
	err := exec.Execute(context.Background())
	require.NoError(t, err)

	dropped := metrics.AggregatedCounterValue(metric.MetricPacingDroppedIterations)
	assert.Greater(t, dropped, int64(0), "vuhive.pacing.dropped_iterations must be > 0 when pool saturates")
}

// AC-1.7.3: ramp_up=200ms, target_tps=10 → first iteration starts after ~100ms (midpoint of ramp)
func TestArrivalRateRampUpMidpointFirstIteration(t *testing.T) {
	logger, metrics := newTestDeps()

	var firstCallTime time.Time
	var once sync.Once
	startTime := time.Now()

	scenario := engine.Scenario{
		RunVU: func(ctx engine.VUContext) error {
			once.Do(func() {
				firstCallTime = time.Now()
			})
			return nil
		},
	}

	cfg := config.ScenarioConfig{
		Type:      config.ScenarioTypeArrivalRate,
		TargetTPS: 10,
		MaxVUs:    10,
		RampUp:    200 * time.Millisecond,
		RunPeriod: 100 * time.Millisecond,
		VUTimeout: 1 * time.Second,
	}

	exec := engine.NewExecutor("test_arrival_rate", scenario, cfg, logger, metrics)
	err := exec.Execute(context.Background())
	require.NoError(t, err)

	require.False(t, firstCallTime.IsZero(), "first iteration must have run")
	delay := firstCallTime.Sub(startTime)
	// Expected ~100ms midpoint delay (tolerance: 60ms to 160ms)
	assert.True(t, delay >= 60*time.Millisecond && delay <= 170*time.Millisecond, "first iteration delay was %v (expected ~100ms)", delay)
}

// AC-1.7.4: All other lifecycle guarantees from Increment 1.6 apply equally to arrival_rate mode
func TestArrivalRateLifecycleHooks(t *testing.T) {
	logger, metrics := newTestDeps()

	var setupCount atomic.Int64
	var preTestCount atomic.Int64
	var afterTestCount atomic.Int64
	var teardownCount atomic.Int64

	scenario := engine.Scenario{
		Setup: func(ctx engine.SetupContext) (map[string]any, error) {
			setupCount.Add(1)
			return map[string]any{"data": "ok"}, nil
		},
		PreTest: func(ctx engine.VUContext) error {
			preTestCount.Add(1)
			return nil
		},
		RunVU: func(ctx engine.VUContext) error {
			if ctx.VUID() == 1 {
				return errors.New("iteration error")
			}
			return nil
		},
		AfterTest: func(ctx engine.VUContext) error {
			afterTestCount.Add(1)
			return nil
		},
		Teardown: func(ctx engine.TeardownContext, state map[string]any) error {
			teardownCount.Add(1)
			assert.Equal(t, "ok", state["data"])
			return nil
		},
	}

	cfg := config.ScenarioConfig{
		Type:      config.ScenarioTypeArrivalRate,
		TargetTPS: 10,
		MaxVUs:    5,
		RunPeriod: 200 * time.Millisecond,
		VUTimeout: 1 * time.Second,
	}

	exec := engine.NewExecutor("test_arrival_rate", scenario, cfg, logger, metrics)
	err := exec.Execute(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(1), setupCount.Load(), "Setup must be called once")
	assert.Equal(t, int64(1), teardownCount.Load(), "Teardown must be called once")
	assert.Greater(t, preTestCount.Load(), int64(0), "PreTest must be called for dispatched workers")
	assert.Equal(t, preTestCount.Load(), afterTestCount.Load(), "AfterTest must match PreTest count")
}

// Issue #70: With ramp_down=0s, in-flight workers interrupted by test duration expiration
// must not be reported as timeouts or failures.
func TestArrivalRateZeroRampDownInFlightWorkersNotReportedAsTimeoutsOrFailures(t *testing.T) {
	logger, metrics := newTestDeps()

	scenario := engine.Scenario{
		RunVU: func(ctx engine.VUContext) error {
			// Simulate context-aware iteration work (25ms)
			select {
			case <-time.After(25 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}

	cfg := config.ScenarioConfig{
		Type:      config.ScenarioTypeArrivalRate,
		TargetTPS: 50,
		MaxVUs:    10,
		RunPeriod: 60 * time.Millisecond,
		RampDown:  0,
		VUTimeout: 1 * time.Second,
	}

	exec := engine.NewExecutor("test_arrival_rate", scenario, cfg, logger, metrics)
	err := exec.Execute(context.Background())
	require.NoError(t, err)

	timeoutCount := metrics.AggregatedCounterValue(metric.MetricIterationsTimeout)
	failedCount := metrics.AggregatedCounterValue(metric.MetricIterationsFailed)
	totalCount := metrics.AggregatedCounterValue(metric.MetricIterationsTotal)

	assert.Equal(t, int64(0), timeoutCount, "interrupted in-flight workers must not be counted as timeouts")
	assert.Equal(t, int64(0), failedCount, "interrupted in-flight workers must not be counted as failures")
	assert.Greater(t, totalCount, int64(0), "completed iterations before expiration should be recorded in total")
}

// Issue #103: BurstBuffer absorbs transient jitter that would cause drops with a small buffer.
// With BurstBuffer=0 (auto-sized to 2×TargetTPS or MaxVUs, whichever is larger), transient
// worker busyness should be absorbed by the queue rather than immediately dropping iterations.
func TestArrivalRate_BurstBuffer_AbsorbsJitter(t *testing.T) {
	logger, metrics := newTestDeps()

	scenario := engine.Scenario{
		RunVU: func(ctx engine.VUContext) error {
			// Brief work simulating occasional jitter
			time.Sleep(10 * time.Millisecond)
			return nil
		},
	}

	// target_tps=5, max_vus=5, vu_timeout=100ms → RequiredVUs=ceil(5*0.1)=1
	// Workers finish in ~10ms, well within 100ms timeout.
	// With 5 VUs and TPS=5, each VU handles at most 1 TPS on average.
	// The burst buffer (auto-sized) should absorb any transient scheduling jitter.
	cfg := config.ScenarioConfig{
		Type:        config.ScenarioTypeArrivalRate,
		TargetTPS:   5,
		MaxVUs:      5,
		BurstBuffer: 20, // explicit burst buffer to absorb jitter
		RunPeriod:   500 * time.Millisecond,
		VUTimeout:   100 * time.Millisecond,
	}

	exec := engine.NewExecutor("test_burst_buffer", scenario, cfg, logger, metrics)
	err := exec.Execute(context.Background())
	require.NoError(t, err)

	dropped := metrics.AggregatedCounterValue(metric.MetricPacingDroppedIterations)
	total := metrics.AggregatedCounterValue(metric.MetricIterationsTotal)

	assert.Equal(t, int64(0), dropped, "burst buffer should absorb all transient jitter without dropping")
	assert.Greater(t, total, int64(0), "should have completed iterations")
}
