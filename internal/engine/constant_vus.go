package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/log"
	"github.com/morphy76/vuhive/internal/metric"
)



// RunConstantVUs executes the constant_vus pacing schedule.
func RunConstantVUs(
	ctx context.Context,
	scenario Scenario,
	cfg config.ScenarioConfig,
	scenarioName string,
	globalState map[string]any,
	logger log.Logger,
	metrics metric.Collector,
) error {
	start := time.Now()
	if logger != nil {
		logger.Debug().
			Str("op", "RunConstantVUs").
			Str("scenario", scenarioName).
			Int("vus", cfg.VUs).
			Msg("starting constant_vus pacing execution")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var gate *StartupQuorumGate
	if cfg.MinReadyRatio > 0 {
		grace := cfg.StartupGracePeriod
		if grace <= 0 {
			grace = cfg.RampUp + 10*time.Second
		}
		gate = NewStartupQuorumGate(cfg.VUs, cfg.MinReadyRatio, grace, logger)
	}

	activeDuration := cfg.RampUp + cfg.RunPeriod + cfg.RampDown
	stopCh := make(chan struct{})

	var wg sync.WaitGroup
	var interval time.Duration
	if cfg.RampUp > 0 && cfg.VUs > 0 {
		interval = cfg.RampUp / time.Duration(cfg.VUs)
	}

	wd := NewExecutionWatchdog(runCtx, cfg, logger, metrics)
	defer wd.Stop()

	for i := 1; i <= cfg.VUs; i++ {
		if i > 1 && interval > 0 {
			select {
			case <-runCtx.Done():
				close(stopCh)
				drainWorkers(runCtx, cancel, &wg, cfg.Drain, logger, metrics)
				return runCtx.Err()
			case <-time.After(interval):
			}
		}

		wg.Add(1)
		vuid := int64(i)
		go runVUGoroutine(runCtx, stopCh, scenario, cfg, scenarioName, vuid, globalState, logger, metrics, gate, wd, &wg)
	}

	// Startup Quorum Gate: await readiness before counting scenario execution time
	if gate != nil {
		if err := gate.AwaitQuorum(runCtx); err != nil {
			cancel()
			close(stopCh)
			drainWorkers(runCtx, cancel, &wg, cfg.Drain, logger, metrics)
			return err
		}
	}

	stopTimer := time.NewTimer(activeDuration)
	defer stopTimer.Stop()

	// Active execution phase: wait until active duration completes or context is cancelled
	select {
	case <-stopTimer.C:
		close(stopCh)
	case <-runCtx.Done():
		close(stopCh)
	}

	// Drain execution phase: wait up to cfg.Drain for remaining in-flight VUs to finish
	drainWorkers(runCtx, cancel, &wg, cfg.Drain, logger, metrics)

	if logger != nil {
		logger.Info().
			Str("op", "RunConstantVUs").
			Str("scenario", scenarioName).
			Dur("duration_ms", time.Since(start)).
			Msg("completed constant_vus pacing execution")
	}
	return nil
}

func runVUGoroutine(
	ctx context.Context,
	stopCh <-chan struct{},
	scenario Scenario,
	cfg config.ScenarioConfig,
	scenarioName string,
	vuid int64,
	globalState map[string]any,
	logger log.Logger,
	metrics metric.Collector,
	gate *StartupQuorumGate,
	wd ExecutionWatchdog,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	activeGauge := metrics.Gauge(metric.MetricVUActive, metric.Tags{})
	activeGauge.Add(1)
	defer activeGauge.Add(-1)

	tracker := wd.RegisterVU(vuid)
	defer tracker.Unregister()

	sCtx := newVUScenarioContext(ctx, vuid, cfg, scenarioName, globalState, logger, metrics)

	// AfterTest is guaranteed to run after PreTest/RunVU exit.
	defer func() {
		if scenario.AfterTest != nil {
			sCtx.prepareIteration(ctx, 0)
			if err := scenario.AfterTest(sCtx); err != nil && logger != nil {
				logger.Error().Err(err).Msg("AfterTest hook error")
			}
		}
	}()

	// Supervised PreTest execution with retry/backoff
	if err := runSupervisedPreTest(ctx, stopCh, scenario, sCtx, cfg, logger, metrics); err != nil {
		if gate != nil {
			gate.RecordFailed(vuid, err)
		}
		return // skips RunVU, deferred AfterTest still runs
	}

	// Startup Quorum Gate synchronization
	if gate != nil {
		gate.RecordReady(vuid)
		if err := gate.WaitReady(ctx); err != nil {
			return
		}
	}

	im := newIterationMetrics(metrics)
	effectiveTimeout, hasTimeout := getEffectiveVUTimeout(cfg)
	var iteration int64
	for {
		select {
		case <-stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		tracker.Begin(iteration)
		if hasTimeout {
			iterCtx, cancel := context.WithTimeout(ctx, effectiveTimeout)
			sCtx.prepareIteration(iterCtx, iteration)
			executeIteration(ctx, iterCtx, sCtx, scenario.RunVU, im, logger)
			cancel()
		} else {
			sCtx.prepareIteration(ctx, iteration)
			executeIteration(ctx, ctx, sCtx, scenario.RunVU, im, logger)
		}
		tracker.End()

		iteration++
	}
}

func executeIteration(
	ctx context.Context,
	iterCtx context.Context,
	sCtx VUContext,
	runVU func(VUContext) error,
	im iterationMetrics,
	logger log.Logger,
) {
	defer func() {
		if r := recover(); r != nil {
			if im.panics != nil {
				im.panics.Inc()
			}
			if im.failed != nil {
				im.failed.Inc()
			}
			if im.total != nil {
				im.total.Inc()
			}
			if logger != nil {
				logger.Error().Str("panic", fmt.Sprintf("%v", r)).Msg("RunVU panicked")
			}
		}
	}()

	err := runVU(sCtx)
	recordIterationResultFast(ctx, iterCtx, err, im, logger)
}



