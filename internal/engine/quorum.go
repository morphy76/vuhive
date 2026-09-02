package engine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/morphy76/vuhive/internal/log"
)

// ErrStartupQuorumFailed is returned when healthy virtual user startup quorum cannot be established.
var ErrStartupQuorumFailed = errors.New("vuhive: startup quorum failed: insufficient healthy virtual users")

// StartupQuorumError details the startup quorum failure parameters.
type StartupQuorumError struct {
	Ready    int
	Target   int
	Required int
	Ratio    float64
	Err      error
}

func (e *StartupQuorumError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("vuhive: startup quorum failed: %d/%d ready (required %d, min_ready_ratio %.2f): %v", e.Ready, e.Target, e.Required, e.Ratio, e.Err)
	}
	return fmt.Sprintf("vuhive: startup quorum failed: %d/%d ready (required %d, min_ready_ratio %.2f)", e.Ready, e.Target, e.Required, e.Ratio)
}

func (e *StartupQuorumError) Unwrap() error {
	return e.Err
}

func (e *StartupQuorumError) Is(target error) bool {
	return target == ErrStartupQuorumFailed
}

var _ error = (*StartupQuorumError)(nil)

// StartupQuorumGate coordinates worker initialization readiness and enforces startup quorum assertions.
type StartupQuorumGate struct {
	target      int
	required    int
	ratio       float64
	gracePeriod time.Duration
	readyCount  atomic.Int64
	failedCount atomic.Int64
	readyCh     chan struct{}
	abortCh     chan struct{}
	readyOnce   sync.Once
	abortOnce   sync.Once
	logger      log.Logger
	errMu       sync.RWMutex
	abortErr    *StartupQuorumError
}

// NewStartupQuorumGate creates a new StartupQuorumGate.
func NewStartupQuorumGate(target int, ratio float64, gracePeriod time.Duration, logger log.Logger) *StartupQuorumGate {
	if target <= 0 {
		target = 1
	}
	required := int(math.Ceil(float64(target) * ratio))
	if required <= 0 {
		required = 1
	}
	if required > target {
		required = target
	}
	if gracePeriod <= 0 {
		gracePeriod = 10 * time.Second
	}

	return &StartupQuorumGate{
		target:      target,
		required:    required,
		ratio:       ratio,
		gracePeriod: gracePeriod,
		readyCh:     make(chan struct{}),
		abortCh:     make(chan struct{}),
		logger:      logger,
	}
}

// RecordReady increments the ready worker count and opens the gate if quorum is met.
func (g *StartupQuorumGate) RecordReady(vuid int64) {
	currentReady := g.readyCount.Add(1)
	if int(currentReady) >= g.required {
		g.readyOnce.Do(func() {
			close(g.readyCh)
			if g.logger != nil {
				g.logger.Info().
					Int("ready_vus", int(currentReady)).
					Int("target_vus", g.target).
					Int("required_vus", g.required).
					Float64("min_ready_ratio", g.ratio).
					Msg("startup quorum reached, releasing virtual users")
			}
		})
	}
}

// RecordFailed increments the failed worker count and triggers abort if quorum is impossible.
func (g *StartupQuorumGate) RecordFailed(vuid int64, err error) {
	currentFailed := g.failedCount.Add(1)
	maxAllowedFailures := g.target - g.required
	if int(currentFailed) > maxAllowedFailures {
		g.triggerAbort(err)
	}
}

func (g *StartupQuorumGate) triggerAbort(err error) {
	g.abortOnce.Do(func() {
		ready := int(g.readyCount.Load())
		qErr := &StartupQuorumError{
			Ready:    ready,
			Target:   g.target,
			Required: g.required,
			Ratio:    g.ratio,
			Err:      err,
		}
		g.errMu.Lock()
		g.abortErr = qErr
		g.errMu.Unlock()
		close(g.abortCh)
		if g.logger != nil {
			g.logger.Error().
				Int("ready_vus", ready).
				Int("target_vus", g.target).
				Int("required_vus", g.required).
				Float64("min_ready_ratio", g.ratio).
				Err(err).
				Msg("startup quorum failed due to excessive dropout")
		}
	})
}

// WaitReady blocks until either the quorum is achieved, aborted, or context is cancelled.
func (g *StartupQuorumGate) WaitReady(ctx context.Context) error {
	select {
	case <-g.readyCh:
		return nil
	case <-g.abortCh:
		g.errMu.RLock()
		defer g.errMu.RUnlock()
		return g.abortErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

// AwaitQuorum is called by the pacing engine orchestrator to wait for startup quorum.
func (g *StartupQuorumGate) AwaitQuorum(ctx context.Context) error {
	timer := time.NewTimer(g.gracePeriod)
	defer timer.Stop()

	select {
	case <-g.readyCh:
		return nil
	case <-g.abortCh:
		g.errMu.RLock()
		defer g.errMu.RUnlock()
		return g.abortErr
	case <-timer.C:
		ready := int(g.readyCount.Load())
		if ready >= g.required {
			return nil
		}
		g.triggerAbort(fmt.Errorf("grace period of %v expired", g.gracePeriod))
		g.errMu.RLock()
		defer g.errMu.RUnlock()
		return g.abortErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
