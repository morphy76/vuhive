package vuhive_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/engine"
	"github.com/morphy76/vuhive/internal/log"
	"github.com/morphy76/vuhive/internal/metric"
	"github.com/morphy76/vuhive/pkg/vuhive"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestAlloc_PublicVUContext_Check(t *testing.T) {
	metrics := metric.NewStore()
	logger := log.New(io.Discard, zerolog.Disabled)
	engCtx := engine.NewScenarioContext(context.Background(), 1, 0, config.ScenarioConfig{}, "test_scenario", nil, logger, metrics)

	suite := vuhive.NewSuite("Alloc Test Suite")
	var capturedCtx vuhive.VUContext
	suite.RegisterScenario("test_scenario", vuhive.Scenario{
		RunVU: func(ctx vuhive.VUContext) error {
			capturedCtx = ctx
			return nil
		},
	})

	runnerAdapter := vuhive.SuiteAdapterForTest(suite)
	engScenario, _ := runnerAdapter.GetScenario("test_scenario")
	_ = engScenario.RunVU(engCtx)

	assert.NotNil(t, capturedCtx)

	// Warm-up check cache
	capturedCtx.Check("status_200", func() string { return "" })
	capturedCtx.CheckEqual("check_eq", 200, 200)
	capturedCtx.CheckTrue("check_true", true)
	capturedCtx.CheckNoError("check_noerror", nil)

	allocsCheck := testing.AllocsPerRun(1000, func() {
		_ = capturedCtx.Check("status_200", func() string { return "" })
	})
	assert.Equal(t, float64(0), allocsCheck, "Public VUContext.Check must produce 0 heap allocations")

	actualCode := 200
	expectedCode := 200
	allocsCheckEqual := testing.AllocsPerRun(1000, func() {
		_ = capturedCtx.CheckEqual("check_eq", actualCode, expectedCode)
	})
	assert.Equal(t, float64(0), allocsCheckEqual, "Public VUContext.CheckEqual must produce 0 heap allocations")

	allocsCheckTrue := testing.AllocsPerRun(1000, func() {
		_ = capturedCtx.CheckTrue("check_true", true)
	})
	assert.Equal(t, float64(0), allocsCheckTrue, "Public VUContext.CheckTrue must produce 0 heap allocations")

	allocsCheckNoError := testing.AllocsPerRun(1000, func() {
		_ = capturedCtx.CheckNoError("check_noerror", nil)
	})
	assert.Equal(t, float64(0), allocsCheckNoError, "Public VUContext.CheckNoError must produce 0 heap allocations")
}

func TestAlloc_PublicVUContext_ParamAccess(t *testing.T) {
	cfg := config.ScenarioConfig{
		Params: map[string]string{
			"url":     "http://localhost:8080/api",
			"retries": "3",
			"timeout": "100ms",
		},
	}
	engCtx := engine.NewScenarioContext(context.Background(), 1, 0, cfg, "test_scenario", nil, nil, nil)

	suite := vuhive.NewSuite("Alloc Test Suite")
	var capturedCtx vuhive.VUContext
	suite.RegisterScenario("test_scenario", vuhive.Scenario{
		RunVU: func(ctx vuhive.VUContext) error {
			capturedCtx = ctx
			return nil
		},
	})

	runnerAdapter := vuhive.SuiteAdapterForTest(suite)
	engScenario, _ := runnerAdapter.GetScenario("test_scenario")
	_ = engScenario.RunVU(engCtx)

	allocs := testing.AllocsPerRun(1000, func() {
		_ = capturedCtx.Param("url")
		_ = capturedCtx.ParamInt("retries", 1)
		_ = capturedCtx.ParamDuration("timeout", time.Second)
	})

	assert.Equal(t, float64(0), allocs, "Public VUContext.Param* must produce 0 heap allocations")
}

func TestAlloc_PublicVUContext_PreResolvedMetrics(t *testing.T) {
	metrics := metric.NewStore()
	logger := log.New(io.Discard, zerolog.Disabled)
	engCtx := engine.NewScenarioContext(context.Background(), 1, 0, config.ScenarioConfig{}, "test_scenario", nil, logger, metrics)

	suite := vuhive.NewSuite("Alloc Test Suite")
	var capturedCtx vuhive.VUContext
	suite.RegisterScenario("test_scenario", vuhive.Scenario{
		RunVU: func(ctx vuhive.VUContext) error {
			capturedCtx = ctx
			return nil
		},
	})

	runnerAdapter := vuhive.SuiteAdapterForTest(suite)
	engScenario, _ := runnerAdapter.GetScenario("test_scenario")
	_ = engScenario.RunVU(engCtx)

	counter := capturedCtx.Metrics().Counter("http_reqs", vuhive.Tags{})
	gauge := capturedCtx.Metrics().Gauge("active_users", vuhive.Tags{})
	duration := capturedCtx.Metrics().Duration("req_dur", vuhive.Tags{})
	rate := capturedCtx.Metrics().Rate("error_rate", vuhive.Tags{})

	allocsCounter := testing.AllocsPerRun(1000, func() {
		counter.Inc()
	})
	assert.Equal(t, float64(0), allocsCounter, "Counter.Inc must produce 0 heap allocations")

	allocsGauge := testing.AllocsPerRun(1000, func() {
		gauge.Set(10)
	})
	assert.Equal(t, float64(0), allocsGauge, "Gauge.Set must produce 0 heap allocations")

	allocsDuration := testing.AllocsPerRun(1000, func() {
		duration.Observe(5 * time.Millisecond)
	})
	assert.Equal(t, float64(0), allocsDuration, "Duration.Observe must produce 0 heap allocations")

	allocsRate := testing.AllocsPerRun(1000, func() {
		rate.Add(1, 1)
	})
	assert.Equal(t, float64(0), allocsRate, "Rate.Add must produce 0 heap allocations")
}

func TestAlloc_Suite_RunVU_AdapterReused(t *testing.T) {
	metrics := metric.NewStore()
	logger := log.New(io.Discard, zerolog.Disabled)
	engCtx := engine.NewScenarioContext(context.Background(), 1, 0, config.ScenarioConfig{}, "test_scenario", nil, logger, metrics)

	suite := vuhive.NewSuite("Alloc Test Suite")
	suite.RegisterScenario("test_scenario", vuhive.Scenario{
		RunVU: func(ctx vuhive.VUContext) error {
			return nil
		},
	})

	runnerAdapter := vuhive.SuiteAdapterForTest(suite)
	engScenario, _ := runnerAdapter.GetScenario("test_scenario")

	// Warm up pool
	_ = engScenario.RunVU(engCtx)

	allocs := testing.AllocsPerRun(1000, func() {
		_ = engScenario.RunVU(engCtx)
	})

	assert.Equal(t, float64(0), allocs, "RunnerSuiteAdapter.RunVU must produce 0 steady-state heap allocations")
}

func TestAlloc_PublicVUContext_Group(t *testing.T) {
	metrics := metric.NewStore()
	logger := log.New(io.Discard, zerolog.Disabled)
	engCtx := engine.NewScenarioContext(context.Background(), 1, 0, config.ScenarioConfig{}, "test_scenario", nil, logger, metrics)

	suite := vuhive.NewSuite("Alloc Test Suite")
	var capturedCtx vuhive.VUContext
	suite.RegisterScenario("test_scenario", vuhive.Scenario{
		RunVU: func(ctx vuhive.VUContext) error {
			capturedCtx = ctx
			return nil
		},
	})

	runnerAdapter := vuhive.SuiteAdapterForTest(suite)
	engScenario, _ := runnerAdapter.GetScenario("test_scenario")
	_ = engScenario.RunVU(engCtx)

	assert.NotNil(t, capturedCtx)

	// Warm-up group cache and adapter pool
	_ = capturedCtx.Group("step1", func(ctx vuhive.VUContext) error {
		return nil
	})

	fn := func(ctx vuhive.VUContext) error {
		return nil
	}

	allocs := testing.AllocsPerRun(1000, func() {
		_ = capturedCtx.Group("step1", fn)
	})

	assert.Equal(t, float64(0), allocs, "Public VUContext.Group must produce 0 steady-state heap allocations")
}

func TestAlloc_PublicVUContext_StateAccessors(t *testing.T) {
	client := &http.Client{}
	state := map[string]any{
		"client": client,
		"retries": 5,
		"url": "http://localhost:8080",
	}
	engCtx := engine.NewScenarioContext(context.Background(), 1, 0, config.ScenarioConfig{}, "test_scenario", state, nil, nil)

	suite := vuhive.NewSuite("Alloc Test Suite")
	var capturedCtx vuhive.VUContext
	suite.RegisterScenario("test_scenario", vuhive.Scenario{
		RunVU: func(ctx vuhive.VUContext) error {
			capturedCtx = ctx
			return nil
		},
	})

	runnerAdapter := vuhive.SuiteAdapterForTest(suite)
	engScenario, _ := runnerAdapter.GetScenario("test_scenario")
	_ = engScenario.RunVU(engCtx)

	assert.NotNil(t, capturedCtx)

	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = vuhive.State[*http.Client](capturedCtx, "client")
		_ = vuhive.MustState[*http.Client](capturedCtx, "client")
		_ = vuhive.StateOrDefault(capturedCtx, "url", "http://fallback")
		_, _ = vuhive.State[int](capturedCtx, "retries")
	})

	assert.Equal(t, float64(0), allocs, "Public VUContext state accessors must produce 0 steady-state heap allocations")
}


