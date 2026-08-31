package vuhive_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/engine"
	"github.com/morphy76/vuhive/internal/log"
	"github.com/morphy76/vuhive/internal/metric"
	"github.com/morphy76/vuhive/pkg/vuhive"
	"github.com/rs/zerolog"
)

func BenchmarkPublicVUContext_Check(b *testing.B) {
	metrics := metric.NewStore()
	logger := log.New(io.Discard, zerolog.Disabled)
	engCtx := engine.NewScenarioContext(context.Background(), 1, 0, config.ScenarioConfig{}, "test_scenario", nil, logger, metrics)

	suite := vuhive.NewSuite("Bench Suite")
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

	checkFn := func() string { return "" }

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		capturedCtx.Check("status_200", checkFn)
	}
}

func BenchmarkPublicVUContext_ParamAccess(b *testing.B) {
	cfg := config.ScenarioConfig{
		Params: map[string]string{
			"url":     "http://localhost:8080/api",
			"retries": "3",
			"timeout": "100ms",
		},
	}
	engCtx := engine.NewScenarioContext(context.Background(), 1, 0, cfg, "test_scenario", nil, nil, nil)

	suite := vuhive.NewSuite("Bench Suite")
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

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = capturedCtx.Param("url")
		_ = capturedCtx.ParamInt("retries", 1)
		_ = capturedCtx.ParamDuration("timeout", time.Second)
	}
}

func BenchmarkPublicVUContext_Metrics_PreResolved(b *testing.B) {
	metrics := metric.NewStore()
	logger := log.New(io.Discard, zerolog.Disabled)
	engCtx := engine.NewScenarioContext(context.Background(), 1, 0, config.ScenarioConfig{}, "test_scenario", nil, logger, metrics)

	suite := vuhive.NewSuite("Bench Suite")
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

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		counter.Inc()
	}
}

func BenchmarkPublicVUContext_Metrics_Duration_PreResolved(b *testing.B) {
	metrics := metric.NewStore()
	logger := log.New(io.Discard, zerolog.Disabled)
	engCtx := engine.NewScenarioContext(context.Background(), 1, 0, config.ScenarioConfig{}, "test_scenario", nil, logger, metrics)

	suite := vuhive.NewSuite("Bench Suite")
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

	duration := capturedCtx.Metrics().Duration("req_duration", vuhive.Tags{})

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		duration.Observe(5 * time.Millisecond)
	}
}

func BenchmarkSuite_RunVU_Iteration(b *testing.B) {
	metrics := metric.NewStore()
	logger := log.New(io.Discard, zerolog.Disabled)
	engCtx := engine.NewScenarioContext(context.Background(), 1, 0, config.ScenarioConfig{}, "test_scenario", nil, logger, metrics)

	suite := vuhive.NewSuite("Bench Suite")
	suite.RegisterScenario("test_scenario", vuhive.Scenario{
		RunVU: func(ctx vuhive.VUContext) error {
			return nil
		},
	})

	runnerAdapter := vuhive.SuiteAdapterForTest(suite)
	engScenario, _ := runnerAdapter.GetScenario("test_scenario")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = engScenario.RunVU(engCtx)
	}
}

func BenchmarkPublicVUContext_StateAccessors(b *testing.B) {
	state := map[string]any{
		"url":     "http://localhost:8080",
		"retries": 5,
	}
	engCtx := engine.NewScenarioContext(context.Background(), 1, 0, config.ScenarioConfig{}, "test_scenario", state, nil, nil)

	suite := vuhive.NewSuite("Bench Suite")
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

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = vuhive.State[string](capturedCtx, "url")
		_ = vuhive.MustState[int](capturedCtx, "retries")
		_ = vuhive.StateOrDefault(capturedCtx, "fallback_key", "default_val")
	}
}

