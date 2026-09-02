package runner

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/engine"
	"github.com/morphy76/vuhive/internal/log"
	"github.com/morphy76/vuhive/internal/metric"
	"github.com/morphy76/vuhive/internal/sla"
	"github.com/rs/zerolog"
)

// Result represents the outcome of running a load test suite.
type Result struct {
	Passed      bool
	Aborted     bool
	AbortReason string
	Error       error
}

// RunSuite executes the suite CLI workflow.
func RunSuite(s ScenarioRegistry, args []string, stdout io.Writer) Result {
	if stdout == nil {
		stdout = io.Discard
	}

	resolved, err := resolveScenario(s, args, stdout)
	if err != nil {
		return Result{Error: err}
	}

	if resolved.ShowVersion {
		return Result{Passed: true}
	}

	// Setup logger
	logLevel, parseErr := zerolog.ParseLevel(resolved.Flags.LogLevel)
	if parseErr != nil {
		logLevel = zerolog.InfoLevel
	}
	// Setup logger with lock-free non-blocking asynchronous diode buffer
	logger := log.NewAsyncWithFormat(stdout, logLevel, resolved.Flags.LogFormat)
	defer func() { _ = logger.Close() }()
	metricsStore := metric.NewStore()
	preRegisterThresholdMetrics(metricsStore, resolved.ScenarioCfg.Thresholds)

	startedAt := time.Now()
	executor := engine.NewExecutor(resolved.TargetScenario, resolved.Scenario, resolved.ScenarioCfg, logger, metricsStore)

	execErr := executor.Execute(context.Background())
	endedAt := time.Now()
	_ = logger.Close()

	if execErr != nil {
		var setupErr *engine.SetupError
		if errors.As(execErr, &setupErr) {
			return Result{Error: setupErr}
		}
		var quorumErr *engine.StartupQuorumError
		if errors.As(execErr, &quorumErr) {
			return Result{Error: quorumErr}
		}
		return Result{Error: execErr}
	}

	// Evaluate strict diagnostics
	var strictDiagnostics []StrictDiagnostic
	strictMode := ResolveStrictMode(
		resolved.Flags.Strict,
		resolved.Flags.StrictFatal,
		resolved.ScenarioCfg.StrictMode,
	)
	if config.IsStrictEnabled(strictMode) {
		unusedParams := engine.UnusedParams(executor.SetupCtx)
		strictDiagnostics = EvaluateStrictDiagnostics(resolved.ScenarioCfg, unusedParams, metricsStore)
		for _, d := range strictDiagnostics {
			logger.Warn().Str("kind", d.Kind).Msg(d.Message)
		}
	}

	// Evaluate SLA thresholds
	thresholdResults := sla.Evaluate(resolved.ScenarioCfg.Thresholds, metricsStore)
	allPassed := sla.AllPassed(thresholdResults)
	if executor.Aborted {
		allPassed = false
	}
	if strictMode == config.StrictModeFatal && len(strictDiagnostics) > 0 {
		allPassed = false
	}

	reportExecution(context.Background(), ReportParams{
		SuiteName:        s.Name(),
		ScenarioName:     resolved.TargetScenario,
		Scenario:         resolved.Scenario,
		ScenarioCfg:      resolved.ScenarioCfg,
		Flags:            resolved.Flags,
		MetricsStore:     metricsStore,
		ThresholdResults: thresholdResults,
		StrictDiagnostics: strictDiagnostics,
		AllPassed:        allPassed,
		Aborted:          executor.Aborted,
		AbortReason:      executor.AbortReason,
		StartedAt:        startedAt,
		EndedAt:          endedAt,
		Stdout:           stdout,
		Logger:           logger.Zerolog(),
	})

	return Result{
		Passed:      allPassed,
		Aborted:     executor.Aborted,
		AbortReason: executor.AbortReason,
		Error:       nil,
	}
}

func preRegisterThresholdMetrics(reg metric.Registry, thresholds []config.ThresholdConfig) {
	if reg == nil {
		return
	}
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
