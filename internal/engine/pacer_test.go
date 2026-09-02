package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/engine"
	"github.com/morphy76/vuhive/internal/log"
	"github.com/morphy76/vuhive/internal/metric"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type customTestPacer struct {
	invoked bool
}

func (c *customTestPacer) Run(
	ctx context.Context,
	scenario engine.Scenario,
	cfg config.ScenarioConfig,
	scenarioName string,
	globalState map[string]any,
	logger log.Logger,
	metrics metric.Collector,
) error {
	c.invoked = true
	return nil
}

func TestPacer_InterfaceSatisfaction(t *testing.T) {
	var _ engine.PacingEngine = (*engine.ConstantVUsPacer)(nil)
	var _ engine.PacingEngine = (*engine.ArrivalRatePacer)(nil)
}

func TestPacingRegistry_RegisterAndGet(t *testing.T) {
	reg := engine.NewPacingRegistry()

	// Built-in pacers
	pConstant, ok := reg.Get(config.ScenarioTypeConstantVUs)
	assert.True(t, ok)
	assert.IsType(t, &engine.ConstantVUsPacer{}, pConstant)

	pArrival, ok := reg.Get(config.ScenarioTypeArrivalRate)
	assert.True(t, ok)
	assert.IsType(t, &engine.ArrivalRatePacer{}, pArrival)

	// Non-existent pacer
	pUnknown, ok := reg.Get(config.ScenarioType("unknown_type"))
	assert.False(t, ok)
	assert.Nil(t, pUnknown)

	// Register custom pacer
	custom := &customTestPacer{}
	reg.Register(config.ScenarioType("custom_type"), custom)

	pCustom, ok := reg.Get(config.ScenarioType("custom_type"))
	assert.True(t, ok)
	assert.Equal(t, custom, pCustom)
}

func TestExecutor_CustomPacingEngineExtension(t *testing.T) {
	logger, metrics := newTestDeps()
	customPacer := &customTestPacer{}

	cfg := config.ScenarioConfig{
		Type:      config.ScenarioType("custom_distributed"),
		RunPeriod: 10 * time.Millisecond,
	}

	scenario := engine.Scenario{}

	exec := engine.NewExecutorWithPacer("custom_scenario", scenario, cfg, logger, metrics, customPacer)
	err := exec.Execute(context.Background())
	require.NoError(t, err)
	assert.True(t, customPacer.invoked, "custom pacer must be invoked without modifying Executor")
}

func TestExecutor_UnsupportedScenarioTypeReturnsError(t *testing.T) {
	logger, metrics := newTestDeps()

	cfg := config.ScenarioConfig{
		Type: config.ScenarioType("completely_unsupported"),
	}

	scenario := engine.Scenario{}

	exec := engine.NewExecutor("unsupported_scenario", scenario, cfg, logger, metrics)
	err := exec.Execute(context.Background())
	require.Error(t, err)
	assert.Equal(t, `vuhive: unsupported scenario type "completely_unsupported"`, err.Error())
}
