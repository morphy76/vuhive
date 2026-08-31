package vuhive_test

import (
	"context"
	"io"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/engine"
	"github.com/morphy76/vuhive/internal/log"
	"github.com/morphy76/vuhive/internal/metric"
	"github.com/morphy76/vuhive/pkg/vuhive"
)

func TestCheck_Passing(t *testing.T) {
	store := metric.NewStore()
	logger := log.New(io.Discard, zerolog.Disabled)
	ctx := engine.NewScenarioContext(
		context.Background(),
		1,
		1,
		config.ScenarioConfig{},
		"test_scenario",
		nil,
		logger,
		store,
	)

	var checkFn vuhive.CheckFunc = func() string {
		return ""
	}

	passed := ctx.Check("status is 200", engine.CheckFunc(checkFn))

	assert.True(t, passed, "passing check should return true")
	assert.Equal(t, int64(1), store.AggregatedCounterValue(vuhive.MetricChecksPassed))
	assert.Equal(t, int64(0), store.AggregatedCounterValue(vuhive.MetricChecksFailed))
}

func TestCheck_Failing(t *testing.T) {
	store := metric.NewStore()
	logger := log.New(io.Discard, zerolog.Disabled)
	ctx := engine.NewScenarioContext(
		context.Background(),
		1,
		1,
		config.ScenarioConfig{},
		"test_scenario",
		nil,
		logger,
		store,
	)

	passed := ctx.Check("status is 200", func() string {
		return "expected 200 OK, got 500 Internal Server Error"
	})

	assert.False(t, passed, "failing check should return false")
	assert.Equal(t, int64(0), store.AggregatedCounterValue(vuhive.MetricChecksPassed))
	assert.Equal(t, int64(1), store.AggregatedCounterValue(vuhive.MetricChecksFailed))
}

func TestCheck_MultipleIndependent(t *testing.T) {
	store := metric.NewStore()
	logger := log.New(io.Discard, zerolog.Disabled)
	ctx := engine.NewScenarioContext(
		context.Background(),
		1,
		1,
		config.ScenarioConfig{},
		"test_scenario",
		nil,
		logger,
		store,
	)

	res1 := ctx.Check("check_1", func() string { return "" })
	res2 := ctx.Check("check_2", func() string { return "fail reason" })
	res3 := ctx.Check("check_3", func() string { return "" })

	assert.True(t, res1)
	assert.False(t, res2)
	assert.True(t, res3)

	assert.Equal(t, int64(2), store.AggregatedCounterValue(vuhive.MetricChecksPassed))
	assert.Equal(t, int64(1), store.AggregatedCounterValue(vuhive.MetricChecksFailed))
}

func TestVUContext_CheckEqual_RecordsCheckMetrics(t *testing.T) {
	store := metric.NewStore()
	logger := log.New(io.Discard, zerolog.Disabled)
	ctx := engine.NewScenarioContext(
		context.Background(),
		1,
		1,
		config.ScenarioConfig{},
		"test_scenario",
		nil,
		logger,
		store,
	)

	pass := ctx.CheckEqual("status_200", 200, 200)
	fail := ctx.CheckEqual("status_400", 400, 200)

	assert.True(t, pass)
	assert.False(t, fail)
	assert.Equal(t, int64(1), store.AggregatedCounterValue(vuhive.MetricChecksPassed))
	assert.Equal(t, int64(1), store.AggregatedCounterValue(vuhive.MetricChecksFailed))
}

func TestVUContext_CheckTrue_RecordsCheckMetrics(t *testing.T) {
	store := metric.NewStore()
	logger := log.New(io.Discard, zerolog.Disabled)
	ctx := engine.NewScenarioContext(
		context.Background(),
		1,
		1,
		config.ScenarioConfig{},
		"test_scenario",
		nil,
		logger,
		store,
	)

	pass := ctx.CheckTrue("is_valid", true)
	fail := ctx.CheckTrue("has_item", false, "item not found")

	assert.True(t, pass)
	assert.False(t, fail)
	assert.Equal(t, int64(1), store.AggregatedCounterValue(vuhive.MetricChecksPassed))
	assert.Equal(t, int64(1), store.AggregatedCounterValue(vuhive.MetricChecksFailed))
}

func TestVUContext_CheckNoError_RecordsCheckMetrics(t *testing.T) {
	store := metric.NewStore()
	logger := log.New(io.Discard, zerolog.Disabled)
	ctx := engine.NewScenarioContext(
		context.Background(),
		1,
		1,
		config.ScenarioConfig{},
		"test_scenario",
		nil,
		logger,
		store,
	)

	pass := ctx.CheckNoError("no_err", nil)
	fail := ctx.CheckNoError("got_err", assert.AnError)

	assert.True(t, pass)
	assert.False(t, fail)
	assert.Equal(t, int64(1), store.AggregatedCounterValue(vuhive.MetricChecksPassed))
	assert.Equal(t, int64(1), store.AggregatedCounterValue(vuhive.MetricChecksFailed))
}

func TestVUContext_Check_WithAssertionGenerators(t *testing.T) {
	store := metric.NewStore()
	logger := log.New(io.Discard, zerolog.Disabled)
	ctx := engine.NewScenarioContext(
		context.Background(),
		1,
		1,
		config.ScenarioConfig{},
		"test_scenario",
		nil,
		logger,
		store,
	)

	suite := vuhive.NewSuite("Check Suite")
	var capturedCtx vuhive.VUContext
	suite.RegisterScenario("test_scenario", vuhive.Scenario{
		RunVU: func(ctx vuhive.VUContext) error {
			capturedCtx = ctx
			return nil
		},
	})

	runnerAdapter := vuhive.SuiteAdapterForTest(suite)
	engScenario, _ := runnerAdapter.GetScenario("test_scenario")
	_ = engScenario.RunVU(ctx)

	assert.NotNil(t, capturedCtx)

	resEqual := capturedCtx.Check("equal_pass", vuhive.Equal(200, 200))
	resTrue := capturedCtx.Check("true_pass", vuhive.True(true))
	resNoError := capturedCtx.Check("noerror_pass", vuhive.NoError(nil))
	resContains := capturedCtx.Check("contains_pass", vuhive.Contains("application/json", "json"))
	resInRange := capturedCtx.Check("inrange_pass", vuhive.InRange(250, 0, 500))

	resDirectEqual := capturedCtx.CheckEqual("direct_equal", "a", "a")
	resDirectTrue := capturedCtx.CheckTrue("direct_true", true)
	resDirectNoError := capturedCtx.CheckNoError("direct_noerror", nil)

	assert.True(t, resEqual)
	assert.True(t, resTrue)
	assert.True(t, resNoError)
	assert.True(t, resContains)
	assert.True(t, resInRange)
	assert.True(t, resDirectEqual)
	assert.True(t, resDirectTrue)
	assert.True(t, resDirectNoError)

	assert.Equal(t, int64(8), store.AggregatedCounterValue(vuhive.MetricChecksPassed))
	assert.Equal(t, int64(0), store.AggregatedCounterValue(vuhive.MetricChecksFailed))
}

