package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/log"
	"github.com/morphy76/vuhive/internal/metric"
	"golang.org/x/time/rate"
)

// RunArrivalRate executes the arrival_rate pacing schedule using a pre-allocated worker pool
// and token bucket dispatch, eliminating per-iteration goroutine spawning overhead.
func RunArrivalRate(
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
			Str("op", "RunArrivalRate").
			Str("scenario", scenarioName).
			Int("target_tps", cfg.TargetTPS).
			Int("max_vus", cfg.MaxVUs).
			Msg("starting arrival_rate pacing execution")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	dispatchDuration := cfg.RampUp + cfg.RunPeriod + cfg.RampDown
	dispatchCtx, dispatchCancel := context.WithTimeout(ctx, dispatchDuration)
	defer dispatchCancel()

	maxVUs := cfg.MaxVUs
	if maxVUs <= 0 {
		maxVUs = 1
	}

	var gate *StartupQuorumGate
	if cfg.MinReadyRatio > 0 {
		grace := cfg.StartupGracePeriod
		if grace <= 0 {
			grace = cfg.RampUp + 10*time.Second
		}
		gate = NewStartupQuorumGate(maxVUs, cfg.MinReadyRatio, grace, logger)
	}

	// Bounded burst buffer: absorbs transient worker availability fluctuations
	// before dropping tokens. When BurstBuffer is explicitly configured, use it;
	// otherwise auto-size to max(2 × TargetTPS, MaxVUs).
	burstBuffer := cfg.BurstBuffer
	if burstBuffer <= 0 {
		burstBuffer = 2 * cfg.TargetTPS
		if burstBuffer < maxVUs {
			burstBuffer = maxVUs
		}
	}
	tokenCh := make(chan int64, burstBuffer)
	var wg sync.WaitGroup
	var iterSeq int64

	closeTokens := sync.OnceFunc(func() {
		close(tokenCh)
	})

	wd := NewExecutionWatchdog(runCtx, cfg, logger, metrics)
	defer wd.Stop()

	// Pre-spawn persistent worker pool of size MaxVUs
	wg.Add(maxVUs)
	for i := 1; i <= maxVUs; i++ {
		vuid := int64(i)
		go runArrivalRateWorkerPool(runCtx, tokenCh, scenario, cfg, scenarioName, vuid, globalState, logger, metrics, gate, wd, &wg)
	}

	// Startup Quorum Gate: await readiness before token dispatch
	if gate != nil {
		if err := gate.AwaitQuorum(runCtx); err != nil {
			cancel()
			closeTokens()
			drainWorkers(runCtx, cancel, &wg, cfg.Drain, logger, metrics)
			return err
		}
	}

	dispatchToken := func() {
		nextIter := atomic.AddInt64(&iterSeq, 1) - 1
		select {
		case tokenCh <- nextIter:
			// Dispatched to an idle worker in the pool
		default:
			// All MaxVUs workers are currently busy executing iterations
			metrics.Counter(metric.MetricPacingDroppedIterations, metric.Tags{}).Inc()
		}
	}

	// 1. Ramp-up phase (if configured)
	if cfg.RampUp > 0 {
		midpoint := cfg.RampUp / 2
		select {
		case <-dispatchCtx.Done():
			closeTokens()
			drainWorkers(runCtx, cancel, &wg, cfg.Drain, logger, metrics)
			return nil
		case <-time.After(midpoint):
			dispatchToken()
		}

		remainingRamp := cfg.RampUp - midpoint
		select {
		case <-dispatchCtx.Done():
			closeTokens()
			drainWorkers(runCtx, cancel, &wg, cfg.Drain, logger, metrics)
			return nil
		case <-time.After(remainingRamp):
		}
	}

	// 2. Steady-state run_period + 3. Ramp-down phase
	limiter := rate.NewLimiter(rate.Limit(cfg.TargetTPS), 1)

steadyLoop:
	for {
		if err := limiter.Wait(dispatchCtx); err != nil {
			break steadyLoop
		}
		dispatchToken()
	}

	// Stop dispatching tokens to workers
	closeTokens()

	// Drain execution phase: wait up to cfg.Drain for remaining in-flight VUs to finish
	drainWorkers(runCtx, cancel, &wg, cfg.Drain, logger, metrics)

	if logger != nil {
		logger.Info().
			Str("op", "RunArrivalRate").
			Str("scenario", scenarioName).
			Dur("duration_ms", time.Since(start)).
			Msg("completed arrival_rate pacing execution")
	}
	return nil
}

func runArrivalRateWorkerPool(
	ctx context.Context,
	tokenCh <-chan int64,
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

	var ap *sync.Map
	if p, ok := ctx.Value(accessedParamsKey{}).(*sync.Map); ok {
		ap = p
	}
	sCtx := newVUScenarioContext(ctx, vuid, cfg, scenarioName, globalState, logger, metrics, ap)

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
	if err := runSupervisedPreTest(ctx, ctx.Done(), scenario, sCtx, cfg, logger, metrics); err != nil {
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
	for {
		select {
		case <-ctx.Done():
			return
		case iteration, ok := <-tokenCh:
			if !ok {
				return
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
		}
	}
}
