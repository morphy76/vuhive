package engine_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/engine"
	"github.com/morphy76/vuhive/internal/metric"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEngine_ConstantVUs_Supervisor_RetriesFailedPreTest verifies that transient PreTest failures
// are retried by the supervisor loop with backoff, restarts_total is recorded, and active VU count recovers.
func TestEngine_ConstantVUs_Supervisor_RetriesFailedPreTest(t *testing.T) {
	logger, metrics := newTestDeps()

	var (
		attempts   atomic.Int64
		runVUCount atomic.Int64
	)

	scenario := engine.Scenario{
		PreTest: func(ctx engine.VUContext) error {
			att := attempts.Add(1)
			// Fail first 2 attempts, succeed on 3rd attempt
			if att <= 2 {
				return errors.New("transient auth handshake failure")
			}
			return nil
		},
		RunVU: func(ctx engine.VUContext) error {
			runVUCount.Add(1)
			time.Sleep(5 * time.Millisecond)
			return nil
		},
	}

	cfg := config.ScenarioConfig{
		Type:              config.ScenarioTypeConstantVUs,
		VUs:               1,
		RunPeriod:         250 * time.Millisecond,
		VUTimeout:         1 * time.Second,
		MaxPreTestRetries: 3,
	}

	exec := engine.NewExecutor("constant_supervisor_test", scenario, cfg, logger, metrics)
	err := exec.Execute(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(3), attempts.Load(), "PreTest should have been attempted 3 times (2 failures + 1 success)")
	assert.Greater(t, runVUCount.Load(), int64(0), "RunVU should have executed after PreTest recovery")

	// Verify metrics
	pretestErrors := metrics.AggregatedCounterValue(metric.MetricVUPretestErrors)
	restartsTotal := metrics.AggregatedCounterValue(metric.MetricVURestartsTotal)
	assert.Equal(t, int64(2), pretestErrors, "pretest_errors should record 2 failed attempts")
	assert.Equal(t, int64(2), restartsTotal, "restarts_total should record 2 supervisor retries")
}

// TestEngine_StartupQuorum_AbortsOnExcessiveDropout verifies fast failure when initialization quorum is not met.
func TestEngine_StartupQuorum_AbortsOnExcessiveDropout(t *testing.T) {
	logger, metrics := newTestDeps()

	var (
		pretestVUs atomic.Int64
		runVUCount atomic.Int64
	)

	scenario := engine.Scenario{
		PreTest: func(ctx engine.VUContext) error {
			pretestVUs.Add(1)
			if ctx.VUID() > 6 {
				// VUs 7, 8, 9, 10 fail permanently
				return errors.New("permanent auth token rejected")
			}
			return nil
		},
		RunVU: func(ctx engine.VUContext) error {
			runVUCount.Add(1)
			return nil
		},
	}

	cfg := config.ScenarioConfig{
		Type:               config.ScenarioTypeConstantVUs,
		VUs:                10,
		RunPeriod:          1 * time.Second,
		VUTimeout:          1 * time.Second,
		MaxPreTestRetries:  1,
		MinReadyRatio:      0.9, // Requires 9/10 VUs to be ready, but only 6 will succeed
		StartupGracePeriod: 500 * time.Millisecond,
	}

	start := time.Now()
	exec := engine.NewExecutor("quorum_dropout_test", scenario, cfg, logger, metrics)
	err := exec.Execute(context.Background())
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.True(t, errors.Is(err, engine.ErrStartupQuorumFailed), "error should match ErrStartupQuorumFailed")

	var quorumErr *engine.StartupQuorumError
	require.True(t, errors.As(err, &quorumErr))
	assert.Equal(t, 10, quorumErr.Target)
	assert.Equal(t, 9, quorumErr.Required)
	assert.Equal(t, 0.9, quorumErr.Ratio)
	assert.LessOrEqual(t, quorumErr.Ready, 6)

	// RunVU should not have executed because quorum failed
	assert.Equal(t, int64(0), runVUCount.Load(), "RunVU iterations must not run when startup quorum fails")
	assert.Less(t, elapsed, 1*time.Second, "Execution should abort fast before full run period")
}

// TestEngine_StartupQuorum_SucceedsWhenQuorumMet verifies that when ready VUs meet or exceed min_ready_ratio,
// the test proceeds and executes normally.
func TestEngine_StartupQuorum_SucceedsWhenQuorumMet(t *testing.T) {
	logger, metrics := newTestDeps()

	var runVUCount atomic.Int64

	scenario := engine.Scenario{
		PreTest: func(ctx engine.VUContext) error {
			if ctx.VUID() == 10 {
				// 1 VU fails permanently out of 10
				return errors.New("single VU token failure")
			}
			return nil
		},
		RunVU: func(ctx engine.VUContext) error {
			runVUCount.Add(1)
			time.Sleep(5 * time.Millisecond)
			return nil
		},
	}

	cfg := config.ScenarioConfig{
		Type:               config.ScenarioTypeConstantVUs,
		VUs:                10,
		RunPeriod:          50 * time.Millisecond,
		VUTimeout:          1 * time.Second,
		MaxPreTestRetries:  1,
		MinReadyRatio:      0.8, // Requires 8/10 VUs to be ready; 9 will succeed
		StartupGracePeriod: 500 * time.Millisecond,
	}

	exec := engine.NewExecutor("quorum_success_test", scenario, cfg, logger, metrics)
	err := exec.Execute(context.Background())
	require.NoError(t, err)
	assert.Greater(t, runVUCount.Load(), int64(0), "RunVU should execute when quorum is reached")
}

// TestEngine_RampingVUs_Supervisor_RetriesFailedPreTest verifies supervisor retries in ramping_vus.
func TestEngine_RampingVUs_Supervisor_RetriesFailedPreTest(t *testing.T) {
	logger, metrics := newTestDeps()

	var (
		attempts   atomic.Int64
		runVUCount atomic.Int64
	)

	scenario := engine.Scenario{
		PreTest: func(ctx engine.VUContext) error {
			att := attempts.Add(1)
			if att == 1 {
				return errors.New("transient network timeout")
			}
			return nil
		},
		RunVU: func(ctx engine.VUContext) error {
			runVUCount.Add(1)
			time.Sleep(5 * time.Millisecond)
			return nil
		},
	}

	cfg := config.ScenarioConfig{
		Type: config.ScenarioTypeRampingVUs,
		Stages: []config.StageConfig{
			{Target: 2, Duration: 50 * time.Millisecond},
		},
		VUTimeout:         1 * time.Second,
		MaxPreTestRetries: 2,
	}

	exec := engine.NewExecutor("ramping_supervisor_test", scenario, cfg, logger, metrics)
	err := exec.Execute(context.Background())
	require.NoError(t, err)

	assert.GreaterOrEqual(t, attempts.Load(), int64(2))
	assert.Greater(t, runVUCount.Load(), int64(0))

	restartsTotal := metrics.AggregatedCounterValue(metric.MetricVURestartsTotal)
	assert.Equal(t, int64(1), restartsTotal)
}

// TestEngine_ArrivalRate_Supervisor_RetriesFailedPreTest verifies supervisor retries in arrival_rate worker pool.
func TestEngine_ArrivalRate_Supervisor_RetriesFailedPreTest(t *testing.T) {
	logger, metrics := newTestDeps()

	var (
		attempts   atomic.Int64
		runVUCount atomic.Int64
	)

	scenario := engine.Scenario{
		PreTest: func(ctx engine.VUContext) error {
			att := attempts.Add(1)
			if att == 1 {
				return errors.New("transient redis connection reset")
			}
			return nil
		},
		RunVU: func(ctx engine.VUContext) error {
			runVUCount.Add(1)
			time.Sleep(5 * time.Millisecond)
			return nil
		},
	}

	cfg := config.ScenarioConfig{
		Type:              config.ScenarioTypeArrivalRate,
		TargetTPS:         10,
		MaxVUs:            2,
		RunPeriod:         250 * time.Millisecond,
		VUTimeout:         1 * time.Second,
		MaxPreTestRetries: 2,
	}

	exec := engine.NewExecutor("arrival_supervisor_test", scenario, cfg, logger, metrics)
	err := exec.Execute(context.Background())
	require.NoError(t, err)

	assert.GreaterOrEqual(t, attempts.Load(), int64(2))
	assert.Greater(t, runVUCount.Load(), int64(0))

	restartsTotal := metrics.AggregatedCounterValue(metric.MetricVURestartsTotal)
	assert.Equal(t, int64(1), restartsTotal)
}
