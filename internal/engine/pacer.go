package engine

import (
	"context"
	"sync"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/log"
	"github.com/morphy76/vuhive/internal/metric"
)

// PacingEngine executes a scenario pacing schedule.
type PacingEngine interface {
	Run(
		ctx context.Context,
		scenario Scenario,
		cfg config.ScenarioConfig,
		scenarioName string,
		globalState map[string]any,
		logger log.Logger,
		metrics metric.Collector,
	) error
}

// ConstantVUsPacer executes the constant_vus pacing strategy.
type ConstantVUsPacer struct{}

// Run executes the constant_vus pacing schedule.
func (p *ConstantVUsPacer) Run(
	ctx context.Context,
	scenario Scenario,
	cfg config.ScenarioConfig,
	scenarioName string,
	globalState map[string]any,
	logger log.Logger,
	metrics metric.Collector,
) error {
	return RunConstantVUs(ctx, scenario, cfg, scenarioName, globalState, logger, metrics)
}

// ArrivalRatePacer executes the arrival_rate pacing strategy.
type ArrivalRatePacer struct{}

// Run executes the arrival_rate pacing schedule.
func (p *ArrivalRatePacer) Run(
	ctx context.Context,
	scenario Scenario,
	cfg config.ScenarioConfig,
	scenarioName string,
	globalState map[string]any,
	logger log.Logger,
	metrics metric.Collector,
) error {
	return RunArrivalRate(ctx, scenario, cfg, scenarioName, globalState, logger, metrics)
}

// RampingVUsPacer executes the ramping_vus pacing strategy.
type RampingVUsPacer struct{}

// Run executes the ramping_vus pacing schedule.
func (p *RampingVUsPacer) Run(
	ctx context.Context,
	scenario Scenario,
	cfg config.ScenarioConfig,
	scenarioName string,
	globalState map[string]any,
	logger log.Logger,
	metrics metric.Collector,
) error {
	return RunRampingVUs(ctx, scenario, cfg, scenarioName, globalState, logger, metrics)
}

// PacingRegistry manages registered PacingEngine implementations indexed by ScenarioType.
type PacingRegistry interface {
	Register(scenarioType config.ScenarioType, pacer PacingEngine)
	Get(scenarioType config.ScenarioType) (PacingEngine, bool)
}

type pacingRegistry struct {
	mu     sync.RWMutex
	pacers map[config.ScenarioType]PacingEngine
}

// NewPacingRegistry creates a new thread-safe PacingRegistry with built-in pacers registered.
func NewPacingRegistry() PacingRegistry {
	r := &pacingRegistry{
		pacers: make(map[config.ScenarioType]PacingEngine),
	}
	r.pacers[config.ScenarioTypeConstantVUs] = &ConstantVUsPacer{}
	r.pacers[config.ScenarioTypeArrivalRate] = &ArrivalRatePacer{}
	r.pacers[config.ScenarioTypeRampingVUs] = &RampingVUsPacer{}
	return r
}

// Register adds or replaces a PacingEngine for the given ScenarioType.
func (r *pacingRegistry) Register(scenarioType config.ScenarioType, pacer PacingEngine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pacers[scenarioType] = pacer
}

// Get retrieves the PacingEngine for the given ScenarioType, or false if not found.
func (r *pacingRegistry) Get(scenarioType config.ScenarioType) (PacingEngine, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	pacer, ok := r.pacers[scenarioType]
	return pacer, ok
}

// DefaultPacingRegistry is the global default registry containing built-in pacers.
var DefaultPacingRegistry = NewPacingRegistry()

// Compile-time interface satisfaction checks.
var (
	_ PacingEngine   = (*ConstantVUsPacer)(nil)
	_ PacingEngine   = (*ArrivalRatePacer)(nil)
	_ PacingEngine   = (*RampingVUsPacer)(nil)
	_ PacingRegistry = (*pacingRegistry)(nil)
)
