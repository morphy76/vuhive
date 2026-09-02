package vuhive_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/morphy76/vuhive/pkg/vuhive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verify that all core domain interfaces and structs can be constructed and interacted with
// directly via pkg/vuhive without any dependency on internal package aliases.

func TestPublicDomainTypesDirectInstantiation(t *testing.T) {
	// Scenario and Hook types
	var setupCalled, preTestCalled, runVUCalled, afterTestCalled, teardownCalled, handleSummaryCalled bool

	sc := vuhive.Scenario{
		Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
			setupCalled = true
			return map[string]any{"token": "xyz"}, nil
		},
		PreTest: func(ctx vuhive.VUContext) error {
			preTestCalled = true
			return nil
		},
		RunVU: func(ctx vuhive.VUContext) error {
			runVUCalled = true
			return nil
		},
		AfterTest: func(ctx vuhive.VUContext) error {
			afterTestCalled = true
			return nil
		},
		Teardown: func(ctx vuhive.TeardownContext, state map[string]any) error {
			teardownCalled = true
			assert.Equal(t, "xyz", state["token"])
			return nil
		},
		HandleSummary: func(ctx vuhive.SummaryContext, summary vuhive.SummaryData) error {
			handleSummaryCalled = true
			assert.Equal(t, "Public Domain Suite", summary.SuiteName)
			return nil
		},
	}

	require.NotNil(t, sc.Setup)
	require.NotNil(t, sc.PreTest)
	require.NotNil(t, sc.RunVU)
	require.NotNil(t, sc.AfterTest)
	require.NotNil(t, sc.Teardown)
	require.NotNil(t, sc.HandleSummary)

	// Execute hooks directly to verify type compatibility
	_, err := sc.Setup(nil)
	assert.NoError(t, err)
	assert.True(t, setupCalled)

	assert.NoError(t, sc.PreTest(nil))
	assert.True(t, preTestCalled)

	assert.NoError(t, sc.RunVU(nil))
	assert.True(t, runVUCalled)

	assert.NoError(t, sc.AfterTest(nil))
	assert.True(t, afterTestCalled)

	assert.NoError(t, sc.Teardown(nil, map[string]any{"token": "xyz"}))
	assert.True(t, teardownCalled)

	assert.NoError(t, sc.HandleSummary(nil, vuhive.SummaryData{SuiteName: "Public Domain Suite"}))
	assert.True(t, handleSummaryCalled)
}

func TestPublicErrorTypes(t *testing.T) {
	t.Run("ConfigError", func(t *testing.T) {
		baseErr := errors.New("file not found")
		cfgErr := &vuhive.ConfigError{Path: "vuhive.yaml", Err: baseErr}
		assert.Equal(t, "vuhive: configuration error in vuhive.yaml: file not found", cfgErr.Error())
		assert.Equal(t, baseErr, cfgErr.Unwrap())

		cfgErrNoPath := &vuhive.ConfigError{Err: baseErr}
		assert.Equal(t, "vuhive: configuration error: file not found", cfgErrNoPath.Error())
	})

	t.Run("ValidationError", func(t *testing.T) {
		valErr := &vuhive.ValidationError{Field: "vus", Message: "must be greater than 0"}
		assert.Equal(t, `vuhive: validation error for field "vus": must be greater than 0`, valErr.Error())
	})

	t.Run("ScenarioNotFoundError", func(t *testing.T) {
		snfErr := &vuhive.ScenarioNotFoundError{Name: "checkout", Message: "not defined"}
		assert.Equal(t, `vuhive: scenario "checkout" not found: not defined`, snfErr.Error())

		snfErrNoName := &vuhive.ScenarioNotFoundError{Message: "no scenario specified"}
		assert.Equal(t, "vuhive: scenario not found: no scenario specified", snfErrNoName.Error())
	})

	t.Run("SetupError", func(t *testing.T) {
		baseErr := errors.New("db connection failed")
		setupErr := &vuhive.SetupError{Err: baseErr}
		assert.Equal(t, "vuhive: setup hook failed: db connection failed", setupErr.Error())
		assert.Equal(t, baseErr, setupErr.Unwrap())
	})

	t.Run("StartupQuorumError", func(t *testing.T) {
		baseErr := errors.New("token acquisition failed")
		quorumErr := &vuhive.StartupQuorumError{
			Ready:    5,
			Target:   10,
			Required: 9,
			Ratio:    0.9,
			Err:      baseErr,
		}
		assert.Equal(t, "vuhive: startup quorum failed: 5/10 ready (required 9, min_ready_ratio 0.90): token acquisition failed", quorumErr.Error())
		assert.Equal(t, baseErr, quorumErr.Unwrap())
		assert.True(t, errors.Is(quorumErr, vuhive.ErrStartupQuorumFailed))

		quorumErrNoErr := &vuhive.StartupQuorumError{
			Ready:    5,
			Target:   10,
			Required: 9,
			Ratio:    0.9,
		}
		assert.Equal(t, "vuhive: startup quorum failed: 5/10 ready (required 9, min_ready_ratio 0.90)", quorumErrNoErr.Error())
	})
}

func TestPublicSummaryDataMethods(t *testing.T) {
	summary := vuhive.SummaryData{
		SuiteName: "E2E Performance",
		Scenario:  "smoke",
		Version:   "1.0.0",
		Commit:    "abc1234",
		StartedAt: time.Now().Add(-10 * time.Second),
		EndedAt:   time.Now(),
		Duration:  10 * time.Second,
		Config:    map[string]any{"vus": 5},
		Metrics: []vuhive.MetricSummary{
			{Name: "req_total", Type: "counter", Count: 100},
			{Name: "active_vus", Type: "gauge", Value: 5.0},
			{Name: "success_rate", Type: "rate", Rate: 0.99},
			{Name: "latency", Type: "duration", Count: 100, P50: 10 * time.Millisecond, P95: 50 * time.Millisecond, P99: 100 * time.Millisecond},
		},
		Checks: []vuhive.CheckSummary{
			{Name: "status_200", Passed: 99, Failed: 1, Total: 100, PassPct: 99.0},
		},
		Thresholds: []vuhive.ThresholdSummary{
			{Metric: "latency", Stat: "p95", Operator: "<=", Target: "100ms", Actual: "50ms", Passed: true},
		},
		Passed:  true,
		Aborted: false,
	}

	assert.Equal(t, int64(100), summary.Counter("req_total"))
	assert.Equal(t, int64(0), summary.Counter("nonexistent"))

	assert.InDelta(t, 5.0, summary.Gauge("active_vus"), 1e-9)
	assert.InDelta(t, 0.0, summary.Gauge("nonexistent"), 1e-9)

	assert.InDelta(t, 0.99, summary.Rate("success_rate"), 1e-9)
	assert.InDelta(t, 0.0, summary.Rate("nonexistent"), 1e-9)

	latMetric := summary.Metric("latency")
	require.NotNil(t, latMetric)
	assert.Equal(t, 50*time.Millisecond, latMetric.P95)
	assert.Nil(t, summary.Metric("nonexistent"))

	th := summary.Threshold("latency")
	require.NotNil(t, th)
	assert.True(t, th.Passed)
	assert.Nil(t, summary.Threshold("nonexistent"))

	jsonBytes, err := summary.JSON()
	require.NoError(t, err)
	assert.Contains(t, string(jsonBytes), `"suite_name": "E2E Performance"`)
	assert.Contains(t, string(jsonBytes), `"status_200"`)
}

func TestPublicDelayGenerators(t *testing.T) {
	// Test DelayStrategy constants
	assert.Equal(t, vuhive.DelayStrategy("fixed"), vuhive.DelayFixed)
	assert.Equal(t, vuhive.DelayStrategy("range"), vuhive.DelayRange)
	assert.Equal(t, vuhive.DelayStrategy("expo"), vuhive.DelayExpo)
	assert.Equal(t, vuhive.DelayStrategy("gaussian"), vuhive.DelayGaussian)

	// FixedDelay
	fGen := vuhive.FixedDelay(50 * time.Millisecond)
	assert.Equal(t, 50*time.Millisecond, fGen.Next())

	// RangeDelay
	rGen := vuhive.RangeDelay(10*time.Millisecond, 20*time.Millisecond)
	d := rGen.Next()
	assert.GreaterOrEqual(t, d, 10*time.Millisecond)
	assert.LessOrEqual(t, d, 20*time.Millisecond)

	// ExpoDelay
	eGen := vuhive.ExpoDelay(100*time.Millisecond, 10*time.Millisecond, 500*time.Millisecond)
	ed := eGen.Next()
	assert.GreaterOrEqual(t, ed, 10*time.Millisecond)
	assert.LessOrEqual(t, ed, 500*time.Millisecond)

	// GaussianDelay
	gGen := vuhive.GaussianDelay(100*time.Millisecond, 20*time.Millisecond, 10*time.Millisecond, 300*time.Millisecond)
	gd := gGen.Next()
	assert.GreaterOrEqual(t, gd, 10*time.Millisecond)
	assert.LessOrEqual(t, gd, 300*time.Millisecond)

	// NewDelayGenerator
	cfg := &vuhive.InteractionDelayConfig{
		Type:     "fixed",
		Duration: 75 * time.Millisecond,
	}
	cfgGen, err := vuhive.NewDelayGenerator(cfg)
	require.NoError(t, err)
	require.NotNil(t, cfgGen)
	assert.Equal(t, 75*time.Millisecond, cfgGen.Next())

	nilGen, err := vuhive.NewDelayGenerator(nil)
	require.NoError(t, err)
	assert.Nil(t, nilGen)

	invalidCfg := &vuhive.InteractionDelayConfig{
		Type: "unknown_strategy",
	}
	_, err = vuhive.NewDelayGenerator(invalidCfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown delay type")
}

func TestCheckFunc(t *testing.T) {
	var passing vuhive.CheckFunc = func() string { return "" }
	var failing vuhive.CheckFunc = func() string { return "expected 200, got 500" }

	assert.Empty(t, passing())
	assert.Equal(t, "expected 200, got 500", failing())
}

func TestThinkTimeConfig_HasNoStructTags(t *testing.T) {
	rt := reflect.TypeOf(vuhive.ThinkTimeConfig{})
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag := f.Tag
		assert.Empty(t, string(tag), "field %s on vuhive.ThinkTimeConfig must not have struct tags, found %q", f.Name, string(tag))
	}
}

