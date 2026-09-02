package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/log"
	"github.com/morphy76/vuhive/internal/metric"
)

// ExecutionWatchdog monitors in-flight VU iterations and detects hung / stalled execution.
type ExecutionWatchdog interface {
	RegisterVU(vuid int64) VUTracker
	Stop()
}

// VUTracker provides zero-allocation start/end markers for active VU iterations.
type VUTracker interface {
	Begin(iteration int64)
	End()
	Unregister()
}

type vuTracker struct {
	vuid      int64
	iteration atomic.Int64
	startNano atomic.Int64
	active    atomic.Bool
	stalled   atomic.Bool
	watchdog  *executionWatchdog
}

func (t *vuTracker) Begin(iteration int64) {
	t.iteration.Store(iteration)
	t.startNano.Store(time.Now().UnixNano())
	t.stalled.Store(false)
	t.active.Store(true)
}

func (t *vuTracker) End() {
	t.active.Store(false)
}

func (t *vuTracker) Unregister() {
	if t.watchdog != nil {
		t.watchdog.unregisterVU(t)
	}
}

type executionWatchdog struct {
	ctx            context.Context
	cancel         context.CancelFunc
	cfg            config.ScenarioConfig
	logger         log.Logger
	metrics        metric.Collector
	aggregator     metric.Aggregator
	stalledCounter metric.Counter
	interval       time.Duration
	threshold      time.Duration
	mu             sync.RWMutex
	trackers       []*vuTracker
	done           chan struct{}
}

// NewExecutionWatchdog creates and starts a background ExecutionWatchdog.
func NewExecutionWatchdog(
	ctx context.Context,
	cfg config.ScenarioConfig,
	logger log.Logger,
	metrics metric.Collector,
) ExecutionWatchdog {
	watchCtx, cancel := context.WithCancel(ctx)

	interval := cfg.WatchdogInterval
	if interval <= 0 {
		interval = config.DefaultWatchdogInterval
	}

	threshold := cfg.WatchdogStallThreshold
	if threshold <= 0 {
		if cfg.VUTimeout > 0 {
			threshold = cfg.VUTimeout
		} else {
			threshold = config.DefaultGuardDeadline
		}
	}

	var aggregator metric.Aggregator
	if agg, ok := metrics.(metric.Aggregator); ok {
		aggregator = agg
	}

	var stalledCounter metric.Counter
	if metrics != nil {
		stalledCounter = metrics.Counter(metric.MetricVUStalledIterations, metric.Tags{})
	}

	ew := &executionWatchdog{
		ctx:            watchCtx,
		cancel:         cancel,
		cfg:            cfg,
		logger:         logger,
		metrics:        metrics,
		aggregator:     aggregator,
		stalledCounter: stalledCounter,
		interval:       interval,
		threshold:      threshold,
		trackers:       make([]*vuTracker, 0, 16),
		done:           make(chan struct{}),
	}

	go ew.run()

	return ew
}

func (ew *executionWatchdog) RegisterVU(vuid int64) VUTracker {
	t := &vuTracker{
		vuid:     vuid,
		watchdog: ew,
	}
	ew.mu.Lock()
	ew.trackers = append(ew.trackers, t)
	ew.mu.Unlock()
	return t
}

func (ew *executionWatchdog) unregisterVU(t *vuTracker) {
	t.active.Store(false)
	ew.mu.Lock()
	defer ew.mu.Unlock()
	for i, existing := range ew.trackers {
		if existing == t {
			ew.trackers = append(ew.trackers[:i], ew.trackers[i+1:]...)
			break
		}
	}
}

func (ew *executionWatchdog) Stop() {
	ew.cancel()
	<-ew.done
}

func (ew *executionWatchdog) run() {
	defer close(ew.done)

	ticker := time.NewTicker(ew.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ew.ctx.Done():
			return
		case <-ticker.C:
			ew.checkStalledIterations()
		}
	}
}

func (ew *executionWatchdog) checkStalledIterations() {
	threshold := ew.threshold
	if ew.cfg.WatchdogStallThreshold <= 0 && ew.aggregator != nil {
		snap := ew.aggregator.MergedHistogramSnapshot(metric.MetricIterationDuration)
		if snap.Count > 0 && snap.P99 > 0 {
			dyn := 3 * snap.P99
			if ew.cfg.VUTimeout > 0 && dyn > ew.cfg.VUTimeout {
				threshold = ew.cfg.VUTimeout
			} else if dyn > 0 {
				threshold = dyn
			}
		}
	}

	now := time.Now().UnixNano()

	ew.mu.RLock()
	trackers := ew.trackers
	for _, t := range trackers {
		if !t.active.Load() {
			continue
		}
		startNano := t.startNano.Load()
		if startNano == 0 {
			continue
		}
		elapsed := time.Duration(now - startNano)
		if elapsed >= threshold {
			if t.stalled.CompareAndSwap(false, true) {
				vuid := t.vuid
				iter := t.iteration.Load()
				if ew.stalledCounter != nil {
					ew.stalledCounter.Inc()
				}
				if ew.logger != nil {
					ew.logger.Warn().
						Int64("vu_id", vuid).
						Int64("iteration", iter).
						Dur("duration", elapsed).
						Dur("threshold", threshold).
						Msg("VU iteration execution stalled / exceeding threshold")
				}
			}
		}
	}
	ew.mu.RUnlock()
}

// Compile-time interface checks.
var (
	_ ExecutionWatchdog = (*executionWatchdog)(nil)
	_ VUTracker         = (*vuTracker)(nil)
)
