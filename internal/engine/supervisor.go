package engine

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/log"
	"github.com/morphy76/vuhive/internal/metric"
)

// runSupervisedPreTest executes a scenario's PreTest hook with supervised retries, exponential backoff, and jitter.
// Returns nil if PreTest succeeded (or was nil). Returns the last PreTest error or context error if exhausted.
func runSupervisedPreTest(
	ctx context.Context,
	stopCh <-chan struct{},
	scenario Scenario,
	sCtx *scenarioContext,
	cfg config.ScenarioConfig,
	logger log.Logger,
	metrics metric.Collector,
) error {
	if scenario.PreTest == nil {
		return nil
	}

	maxRetries := cfg.MaxPreTestRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	baseBackoff := 10 * time.Millisecond
	maxBackoff := 1 * time.Second

	for attempt := 0; ; attempt++ {
		sCtx.prepareIteration(ctx, 0)
		err := executePreTestWithRecover(sCtx, scenario.PreTest, logger)
		if err == nil {
			return nil
		}

		metrics.Counter(metric.MetricVUPretestErrors, metric.Tags{}).Inc()

		if attempt >= maxRetries {
			if logger != nil {
				logger.Error().
					Err(err).
					Int64("vu_id", sCtx.VUID()).
					Int("attempts", attempt+1).
					Int("max_retries", maxRetries).
					Msg("PreTest hook failed permanently after exhausting all retries, skipping RunVU")
			}
			return err
		}

		// Record restart/retry telemetry
		metrics.Counter(metric.MetricVURestartsTotal, metric.Tags{}).Inc()

		// Exponential backoff with jitter
		delay := baseBackoff * (1 << attempt)
		if delay > maxBackoff || delay <= 0 {
			delay = maxBackoff
		}
		// Add up to 50% random jitter
		jitter := time.Duration(rand.Int64N(int64(delay/2) + 1))
		sleepDuration := delay + jitter

		if logger != nil {
			logger.Warn().
				Err(err).
				Int64("vu_id", sCtx.VUID()).
				Int("retry_attempt", attempt+1).
				Int("max_retries", maxRetries).
				Dur("backoff", sleepDuration).
				Msg("PreTest hook failed, supervisor scheduling retry with backoff")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stopCh:
			return context.Canceled
		case <-time.After(sleepDuration):
		}
	}
}

func executePreTestWithRecover(sCtx VUContext, preTest PreTestHook, logger log.Logger) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pretest panic: %v", r)
			if logger != nil {
				logger.Error().Str("panic", fmt.Sprintf("%v", r)).Msg("PreTest hook panicked")
			}
		}
	}()
	return preTest(sCtx)
}
