package engine

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/log"
	"github.com/morphy76/vuhive/internal/metric"
)

type rampingVUWorker struct {
	id     int64
	stopCh chan struct{}
}

// RunRampingVUs executes the ramping_vus multi-stage pacing schedule.
func RunRampingVUs(
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
			Str("op", "RunRampingVUs").
			Str("scenario", scenarioName).
			Int("stages_count", len(cfg.Stages)).
			Msg("starting ramping_vus pacing execution")
	}

	var totalStagesDuration time.Duration
	for _, st := range cfg.Stages {
		totalStagesDuration += st.Duration
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var gate *StartupQuorumGate
	if cfg.MinReadyRatio > 0 && len(cfg.Stages) > 0 && cfg.Stages[0].Target > 0 {
		grace := cfg.StartupGracePeriod
		if grace <= 0 {
			grace = cfg.Stages[0].Duration + 10*time.Second
		}
		gate = NewStartupQuorumGate(cfg.Stages[0].Target, cfg.MinReadyRatio, grace, logger)
	}

	var (
		wg       sync.WaitGroup
		vuidSeq  int64
		workers  []*rampingVUWorker
		activeMu sync.Mutex
	)

	adjustWorkers := func(desiredVUs int, wGate *StartupQuorumGate) {
		activeMu.Lock()
		defer activeMu.Unlock()

		if desiredVUs < 0 {
			desiredVUs = 0
		}

		currentCount := len(workers)
		if desiredVUs > currentCount {
			toSpawn := desiredVUs - currentCount
			for i := 0; i < toSpawn; i++ {
				vuidSeq++
				w := &rampingVUWorker{
					id:     vuidSeq,
					stopCh: make(chan struct{}),
				}
				workers = append(workers, w)
				wg.Add(1)
				go runRampingVUGoroutine(runCtx, w.stopCh, scenario, cfg, scenarioName, w.id, globalState, logger, metrics, wGate, &wg)
			}
		} else if desiredVUs < currentCount {
			toStop := currentCount - desiredVUs
			for i := currentCount - 1; i >= currentCount-toStop; i-- {
				close(workers[i].stopCh)
			}
			workers = workers[:desiredVUs]
		}
	}

	// Initial worker ramp/startup if stage 0 target > 0
	if len(cfg.Stages) > 0 && cfg.Stages[0].Target > 0 && gate != nil {
		adjustWorkers(cfg.Stages[0].Target, gate)
		if err := gate.AwaitQuorum(runCtx); err != nil {
			cancel()
			activeMu.Lock()
			for _, w := range workers {
				close(w.stopCh)
			}
			workers = nil
			activeMu.Unlock()
			drainWorkers(runCtx, cancel, &wg, cfg.Drain, logger, metrics)
			return err
		}
	}

	startVU := 0
stagesLoop:
	for _, stage := range cfg.Stages {
		select {
		case <-runCtx.Done():
			break stagesLoop
		default:
		}

		targetVU := stage.Target
		stageDuration := stage.Duration
		stageStart := time.Now()

		tickInterval := 50 * time.Millisecond
		if stageDuration <= tickInterval {
			tickInterval = stageDuration / 4
			if tickInterval <= 0 {
				tickInterval = time.Millisecond
			}
		}

		ticker := time.NewTicker(tickInterval)
		stageTimer := time.NewTimer(stageDuration)
		stageDone := false

		for !stageDone {
			select {
			case <-runCtx.Done():
				ticker.Stop()
				stageTimer.Stop()
				stageDone = true
			case <-stageTimer.C:
				ticker.Stop()
				adjustWorkers(targetVU, nil)
				stageDone = true
			case <-ticker.C:
				elapsed := time.Since(stageStart)
				if elapsed >= stageDuration {
					adjustWorkers(targetVU, nil)
					stageDone = true
				} else {
					progress := float64(elapsed) / float64(stageDuration)
					desired := float64(startVU) + progress*float64(targetVU-startVU)
					desiredCount := int(math.Round(desired))
					adjustWorkers(desiredCount, nil)
				}
			}
		}

		startVU = targetVU
	}

	// RampDown period (if configured and context not cancelled)
	if cfg.RampDown > 0 {
		select {
		case <-runCtx.Done():
		case <-time.After(cfg.RampDown):
		}
	}

	// Signal any remaining active workers to stop starting new iterations
	activeMu.Lock()
	for _, w := range workers {
		close(w.stopCh)
	}
	workers = nil
	activeMu.Unlock()

	// Drain execution phase: wait up to cfg.Drain for remaining in-flight VUs to finish
	drainWorkers(runCtx, cancel, &wg, cfg.Drain, logger, metrics)

	if logger != nil {
		logger.Info().
			Str("op", "RunRampingVUs").
			Str("scenario", scenarioName).
			Dur("duration_ms", time.Since(start)).
			Msg("completed ramping_vus pacing execution")
	}
	return nil
}

func runRampingVUGoroutine(
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
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	activeGauge := metrics.Gauge(metric.MetricVUActive, metric.Tags{})
	activeGauge.Add(1)
	defer activeGauge.Add(-1)

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
	hasTimeout := cfg.VUTimeout > 0
	var iteration int64
	for {
		select {
		case <-stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		if hasTimeout {
			iterCtx, cancel := context.WithTimeout(ctx, cfg.VUTimeout)
			sCtx.prepareIteration(iterCtx, iteration)
			executeIteration(ctx, iterCtx, sCtx, scenario.RunVU, im, logger)
			cancel()
		} else {
			sCtx.prepareIteration(ctx, iteration)
			executeIteration(ctx, ctx, sCtx, scenario.RunVU, im, logger)
		}

		iteration++
	}
}
