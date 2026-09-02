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

func TestRunRampingVUs_MultiStageLifecycle(t *testing.T) {
	logger, metrics := newTestDeps()

	var (
		preTestVUs   sync.Map
		afterTestVUs sync.Map
		iterCount    int64
		maxActiveVU  int64
	)

	scenario := engine.Scenario{
		PreTest: func(ctx engine.VUContext) error {
			preTestVUs.Store(ctx.VUID(), true)
			return nil
		},
		RunVU: func(ctx engine.VUContext) error {
			atomic.AddInt64(&iterCount, 1)
			active := metrics.LastGaugeValue(metric.MetricVUActive)
			for {
				currentMax := atomic.LoadInt64(&maxActiveVU)
				if int64(active) <= currentMax {
					break
				}
				if atomic.CompareAndSwapInt64(&maxActiveVU, currentMax, int64(active)) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			return nil
		},
		AfterTest: func(ctx engine.VUContext) error {
			afterTestVUs.Store(ctx.VUID(), true)
			return nil
		},
	}

	cfg := config.ScenarioConfig{
		Type:      config.ScenarioTypeRampingVUs,
		VUTimeout: 500 * time.Millisecond,
		Stages: []config.StageConfig{
			{Target: 2, Duration: 60 * time.Millisecond},
			{Target: 4, Duration: 60 * time.Millisecond},
			{Target: 0, Duration: 60 * time.Millisecond},
		},
	}

	start := time.Now()
	err := engine.RunRampingVUs(context.Background(), scenario, cfg, "spike_test", nil, logger, metrics)
	require.NoError(t, err)
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, elapsed, 150*time.Millisecond)
	assert.Greater(t, atomic.LoadInt64(&iterCount), int64(0))
	assert.GreaterOrEqual(t, atomic.LoadInt64(&maxActiveVU), int64(2))

	// All spawned VUs that ran PreTest should also have run AfterTest
	preCount := 0
	preTestVUs.Range(func(key, value any) bool {
		preCount++
		_, ok := afterTestVUs.Load(key)
		assert.True(t, ok, "VU %v should have run AfterTest", key)
		return true
	})
	assert.Greater(t, preCount, 0)

	// Final active VUs gauge should be 0
	assert.Equal(t, float64(0), metrics.LastGaugeValue(metric.MetricVUActive))
	assert.Greater(t, metrics.AggregatedCounterValue(metric.MetricIterationsTotal), int64(0))
}

func TestRunRampingVUs_PreTestError(t *testing.T) {
	logger, metrics := newTestDeps()

	var afterCalled atomic.Bool
	scenario := engine.Scenario{
		PreTest: func(ctx engine.VUContext) error {
			return errors.New("pretest setup failure")
		},
		RunVU: func(ctx engine.VUContext) error {
			t.Fatal("RunVU should not be called on PreTest error")
			return nil
		},
		AfterTest: func(ctx engine.VUContext) error {
			afterCalled.Store(true)
			return nil
		},
	}

	cfg := config.ScenarioConfig{
		Type:      config.ScenarioTypeRampingVUs,
		VUTimeout: 100 * time.Millisecond,
		Stages: []config.StageConfig{
			{Target: 1, Duration: 50 * time.Millisecond},
		},
	}

	_ = engine.RunRampingVUs(context.Background(), scenario, cfg, "pretest_err_test", nil, logger, metrics)

	assert.True(t, afterCalled.Load())
	assert.Equal(t, int64(1), metrics.AggregatedCounterValue(metric.MetricVUPretestErrors))
	assert.Equal(t, int64(0), metrics.AggregatedCounterValue(metric.MetricIterationsTotal))
}

func TestRunRampingVUs_PanicRecovery(t *testing.T) {
	logger, metrics := newTestDeps()

	scenario := engine.Scenario{
		RunVU: func(ctx engine.VUContext) error {
			panic("unexpected failure in VU execution")
		},
	}

	cfg := config.ScenarioConfig{
		Type:      config.ScenarioTypeRampingVUs,
		VUTimeout: 100 * time.Millisecond,
		Stages: []config.StageConfig{
			{Target: 2, Duration: 60 * time.Millisecond},
			{Target: 2, Duration: 60 * time.Millisecond},
		},
	}

	require.NotPanics(t, func() {
		_ = engine.RunRampingVUs(context.Background(), scenario, cfg, "panic_test", nil, logger, metrics)
	})

	assert.Greater(t, metrics.AggregatedCounterValue(metric.MetricVUPanics), int64(0))
	assert.Greater(t, metrics.AggregatedCounterValue(metric.MetricIterationsFailed), int64(0))
}

func TestRunRampingVUs_ContextCancellation(t *testing.T) {
	logger, metrics := newTestDeps()

	scenario := engine.Scenario{
		RunVU: func(ctx engine.VUContext) error {
			time.Sleep(50 * time.Millisecond)
			return nil
		},
	}

	cfg := config.ScenarioConfig{
		Type:      config.ScenarioTypeRampingVUs,
		VUTimeout: 500 * time.Millisecond,
		Stages: []config.StageConfig{
			{Target: 10, Duration: 5 * time.Second},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_ = engine.RunRampingVUs(ctx, scenario, cfg, "cancel_test", nil, logger, metrics)
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 1*time.Second)
	assert.Equal(t, float64(0), metrics.LastGaugeValue(metric.MetricVUActive))
}

func TestRampingVUsPacer_ExecutorIntegration(t *testing.T) {
	registry := engine.NewPacingRegistry()
	pacer, ok := registry.Get(config.ScenarioTypeRampingVUs)
	require.True(t, ok)
	require.NotNil(t, pacer)

	logger, metrics := newTestDeps()

	var (
		setupCalled    bool
		teardownCalled bool
		iterCount      int64
	)

	scenario := engine.Scenario{
		Setup: func(ctx engine.SetupContext) (map[string]any, error) {
			setupCalled = true
			return map[string]any{"token": "secret"}, nil
		},
		RunVU: func(ctx engine.VUContext) error {
			assert.Equal(t, "secret", ctx.GlobalState("token"))
			atomic.AddInt64(&iterCount, 1)
			time.Sleep(5 * time.Millisecond)
			return nil
		},
		Teardown: func(ctx engine.TeardownContext, state map[string]any) error {
			teardownCalled = true
			assert.Equal(t, "secret", state["token"])
			return nil
		},
	}

	cfg := config.ScenarioConfig{
		Type:      config.ScenarioTypeRampingVUs,
		VUTimeout: 200 * time.Millisecond,
		Stages: []config.StageConfig{
			{Target: 2, Duration: 30 * time.Millisecond},
			{Target: 0, Duration: 30 * time.Millisecond},
		},
	}

	exec := engine.NewExecutor("ramping_integ", scenario, cfg, logger, metrics)
	err := exec.Execute(context.Background())

	require.NoError(t, err)
	assert.True(t, setupCalled)
	assert.True(t, teardownCalled)
	assert.Greater(t, atomic.LoadInt64(&iterCount), int64(0))
}
