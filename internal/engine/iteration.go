package engine

import (
	"context"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/log"
	"github.com/morphy76/vuhive/internal/metric"
)

// iterationMetrics holds pre-resolved built-in counter handles for zero-lookup iteration recording.
type iterationMetrics struct {
	total   metric.Counter
	failed  metric.Counter
	timeout metric.Counter
	panics  metric.Counter
}

func newIterationMetrics(metrics metric.Collector) iterationMetrics {
	if metrics == nil {
		return iterationMetrics{}
	}
	emptyTags := metric.Tags{}
	return iterationMetrics{
		total:   metrics.Counter(metric.MetricIterationsTotal, emptyTags),
		failed:  metrics.Counter(metric.MetricIterationsFailed, emptyTags),
		timeout: metrics.Counter(metric.MetricIterationsTimeout, emptyTags),
		panics:  metrics.Counter(metric.MetricVUPanics, emptyTags),
	}
}

// recordIterationResult handles metrics and logging for a completed or interrupted RunVU iteration.
// It differentiates between:
// 1. Scenario completion / cancellation (parent ctx.Err() != nil):
//    - If err == nil: iteration finished cleanly before/at completion -> increment MetricIterationsTotal.
//    - If err != nil: iteration was interrupted mid-flight by scenario shutdown -> do not count as timeout or failure.
// 2. Normal execution (parent ctx.Err() == nil):
//    - If iterCtx.Err() == context.DeadlineExceeded: genuine per-iteration VUTimeout -> count as timeout, failure, total.
//    - If err != nil: application/scenario error -> count as failure, total.
//    - If err == nil: success -> count as total.
func recordIterationResult(
	ctx context.Context,
	iterCtx context.Context,
	err error,
	metrics metric.Collector,
	logger log.Logger,
) {
	recordIterationResultFast(ctx, iterCtx, err, newIterationMetrics(metrics), logger)
}

func recordIterationResultFast(
	ctx context.Context,
	iterCtx context.Context,
	err error,
	im iterationMetrics,
	logger log.Logger,
) {
	if ctx.Err() != nil {
		// Scenario lifecycle context was cancelled or expired.
		if err == nil {
			if im.total != nil {
				im.total.Inc()
			}
		} else if logger != nil {
			logger.Debug().Err(err).Msg("RunVU iteration interrupted by scenario completion")
		}
		return
	}

	if iterCtx.Err() == context.DeadlineExceeded {
		if im.timeout != nil {
			im.timeout.Inc()
		}
		if im.failed != nil {
			im.failed.Inc()
		}
		if im.total != nil {
			im.total.Inc()
		}
		if logger != nil {
			logger.Error().Err(iterCtx.Err()).Msg("RunVU iteration timed out")
		}
	} else if err != nil {
		if im.failed != nil {
			im.failed.Inc()
		}
		if im.total != nil {
			im.total.Inc()
		}
		if logger != nil {
			logger.Error().Err(err).Msg("RunVU returned error")
		}
	} else {
		if im.total != nil {
			im.total.Inc()
		}
	}
}

// getEffectiveVUTimeout determines the active iteration timeout duration.
// If VUTimeout > 0, returns (VUTimeout, true).
// If AllowUnboundedIterations is true and VUTimeout == 0, returns (0, false).
// Otherwise, returns (DefaultGuardDeadline 30s, true).
func getEffectiveVUTimeout(cfg config.ScenarioConfig) (time.Duration, bool) {
	if cfg.VUTimeout > 0 {
		return cfg.VUTimeout, true
	}
	if cfg.AllowUnboundedIterations {
		return 0, false
	}
	return config.DefaultGuardDeadline, true
}
