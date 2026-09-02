package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/log"
	"github.com/morphy76/vuhive/internal/metric"
	"github.com/morphy76/vuhive/internal/sla"
)

// MetricsStore defines the metric recording and reading capabilities required by the scenario executor.
type MetricsStore interface {
	metric.Collector
	sla.MetricReader
}

var _ MetricsStore = (*metric.Store)(nil)

// Executor orchestrates scenario execution: Setup -> VUs -> Teardown.
type Executor struct {
	ScenarioName string
	Scenario     Scenario
	Config       config.ScenarioConfig
	Logger       log.Logger
	Metrics      MetricsStore
	Pacer        PacingEngine
	Aborted      bool
	AbortReason  string
}

// NewExecutor creates a new scenario executor resolving the pacing engine from DefaultPacingRegistry.
func NewExecutor(
	scenarioName string,
	scenario Scenario,
	cfg config.ScenarioConfig,
	logger log.Logger,
	metrics MetricsStore,
) *Executor {
	preRegisterThresholdMetrics(metrics, cfg.Thresholds)
	pacer, _ := DefaultPacingRegistry.Get(cfg.Type)
	return &Executor{
		ScenarioName: scenarioName,
		Scenario:     scenario,
		Config:       cfg,
		Logger:       logger,
		Metrics:      metrics,
		Pacer:        pacer,
	}
}

// NewExecutorWithPacer creates a new scenario executor with an explicitly injected pacing engine.
func NewExecutorWithPacer(
	scenarioName string,
	scenario Scenario,
	cfg config.ScenarioConfig,
	logger log.Logger,
	metrics MetricsStore,
	pacer PacingEngine,
) *Executor {
	preRegisterThresholdMetrics(metrics, cfg.Thresholds)
	return &Executor{
		ScenarioName: scenarioName,
		Scenario:     scenario,
		Config:       cfg,
		Logger:       logger,
		Metrics:      metrics,
		Pacer:        pacer,
	}
}

func preRegisterThresholdMetrics(metrics MetricsStore, thresholds []config.ThresholdConfig) {
	if reg, ok := metrics.(metric.Registry); ok && reg != nil {
		for _, th := range thresholds {
			if th.Metric == "" {
				continue
			}
			if config.IsDurationStat(th.Stat) {
				_ = reg.Register(th.Metric, metric.MetricTypeDuration)
			} else {
				switch th.Stat {
				case "count":
					_ = reg.Register(th.Metric, metric.MetricTypeCounter)
				case "rate":
					_ = reg.Register(th.Metric, metric.MetricTypeRate)
				case "value":
					_ = reg.Register(th.Metric, metric.MetricTypeGauge)
				}
			}
		}
	}
}

// Execute runs the complete scenario lifecycle.
func (e *Executor) Execute(ctx context.Context) error {
	var globalState map[string]any

	// 1. Setup phase
	if e.Scenario.Setup != nil {
		setupCtx := newScenarioContext(ctx, 0, 0, e.Config, e.ScenarioName, nil, e.Logger, e.Metrics)
		var err error
		globalState, err = e.Scenario.Setup(setupCtx)
		if err != nil {
			return &SetupError{Err: err}
		}
		// Shallow copy the state map so VUs cannot mutate the keys of the original map (spec §4.3).
		// NOTE ON SHALLOW COPY LIMITATION:
		// The shallow copy creates a new map containing the same values returned by Setup.
		// It does NOT deep-copy nested mutable objects (slices, maps, pointer structs).
		// If Setup returns shared mutable objects, concurrent VU access may cause data races.
		// Complex or nested structures stored in globalState must either be immutable or protected
		// with appropriate synchronization (e.g., sync.RWMutex, atomic types, or thread-safe containers).
		if globalState != nil {
			safeCopy := make(map[string]any, len(globalState))
			for k, v := range globalState {
				safeCopy[k] = v
			}
			globalState = safeCopy
		}
	}

	// 2. VU Pacing Engine phase with abort monitor
	pacingCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	startTime := time.Now()
	abortedCh, getReason := MonitorAbortThresholds(pacingCtx, cancel, startTime, e.Config.Thresholds, e.Metrics, e.Logger)

	if e.Pacer == nil {
		return fmt.Errorf("vuhive: unsupported scenario type %q", e.Config.Type)
	}

	pacerErr := e.Pacer.Run(pacingCtx, e.Scenario, e.Config, e.ScenarioName, globalState, e.Logger, e.Metrics)
	if pacerErr != nil {
		if e.Scenario.Teardown != nil {
			teardownCtx := newScenarioContext(ctx, 0, 0, e.Config, e.ScenarioName, globalState, e.Logger, e.Metrics)
			if err := e.Scenario.Teardown(teardownCtx, globalState); err != nil && e.Logger != nil {
				e.Logger.Error().Err(err).Msg("Teardown hook error")
			}
		}
		return pacerErr
	}

	select {
	case <-abortedCh:
		e.Aborted = true
		e.AbortReason = getReason()
	default:
		if reason := getReason(); reason != "" {
			e.Aborted = true
			e.AbortReason = reason
		}
	}

	// 3. Teardown phase
	if e.Scenario.Teardown != nil {
		teardownCtx := newScenarioContext(ctx, 0, 0, e.Config, e.ScenarioName, globalState, e.Logger, e.Metrics)
		if err := e.Scenario.Teardown(teardownCtx, globalState); err != nil {
			e.Logger.Error().Err(err).Msg("Teardown hook error")
		}
	}

	return nil
}
