package runner_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/morphy76/vuhive/internal/cli"
	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/engine"
	"github.com/morphy76/vuhive/internal/metric"
	"github.com/morphy76/vuhive/internal/report"
	"github.com/morphy76/vuhive/internal/runner"
	"github.com/morphy76/vuhive/internal/sla"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRegistry implements runner.ScenarioRegistry for tests
type mockRegistry struct {
	name      string
	scenarios map[string]engine.Scenario
}

func newMockRegistry(name string) *mockRegistry {
	return &mockRegistry{
		name:      name,
		scenarios: make(map[string]engine.Scenario),
	}
}

func (m *mockRegistry) Name() string {
	return m.name
}

func (m *mockRegistry) GetScenario(name string) (engine.Scenario, bool) {
	s, ok := m.scenarios[name]
	return s, ok
}

func (m *mockRegistry) Register(name string, s engine.Scenario) {
	m.scenarios[name] = s
}

// helper to create a temporary config file for tests
func createTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "vuhive.yaml")
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
	return path
}

func TestScenarioNotFoundError_ErrorFormatting(t *testing.T) {
	errWithName := &runner.ScenarioNotFoundError{
		Name:    "checkout",
		Message: "not found in config",
	}
	assert.Equal(t, `vuhive: scenario "checkout" not found: not found in config`, errWithName.Error())

	errWithoutName := &runner.ScenarioNotFoundError{
		Name:    "",
		Message: "no scenario specified",
	}
	assert.Equal(t, "vuhive: scenario not found: no scenario specified", errWithoutName.Error())
}

func TestScenarioResolver_ShowVersion(t *testing.T) {
	reg := newMockRegistry("test-suite")
	resolver := runner.NewScenarioResolver(reg)

	var stdout bytes.Buffer
	res, err := resolver.Resolve([]string{"--version"}, &stdout)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.ShowVersion)
	assert.Contains(t, stdout.String(), "vuhive version")
}

func TestScenarioResolver_FlagParseError(t *testing.T) {
	reg := newMockRegistry("test-suite")
	resolver := runner.NewScenarioResolver(reg)

	var stdout bytes.Buffer
	res, err := resolver.Resolve([]string{"--unknown-flag=foo"}, &stdout)
	require.Error(t, err)
	assert.Nil(t, res)

	var cfgErr *config.ConfigError
	assert.True(t, errors.As(err, &cfgErr))
}

func TestScenarioResolver_ConfigNotFound(t *testing.T) {
	reg := newMockRegistry("test-suite")
	resolver := runner.NewScenarioResolver(reg)

	var stdout bytes.Buffer
	res, err := resolver.Resolve([]string{"--config=/nonexistent/path/vuhive.yaml"}, &stdout)
	require.Error(t, err)
	assert.Nil(t, res)

	var cfgErr *config.ConfigError
	require.True(t, errors.As(err, &cfgErr))
	assert.Equal(t, "/nonexistent/path/vuhive.yaml", cfgErr.Path)
}

func TestScenarioResolver_ConfigValidationError(t *testing.T) {
	cfgFile := createTempConfig(t, `
version: "1.0"
scenarios:
  test_scenario:
    type: invalid_profile
    run_period: 10s
    vu_timeout: 1s
`)
	reg := newMockRegistry("test-suite")
	resolver := runner.NewScenarioResolver(reg)

	var stdout bytes.Buffer
	res, err := resolver.Resolve([]string{"--config=" + cfgFile}, &stdout)
	require.Error(t, err)
	assert.Nil(t, res)

	var valErr *config.ValidationError
	assert.True(t, errors.As(err, &valErr))
}

func TestScenarioResolver_NoScenarioSpecified(t *testing.T) {
	cfgFile := createTempConfig(t, `
version: "1.0"
scenarios:
  test_scenario:
    type: constant_vus
    vus: 1
    run_period: 1s
    vu_timeout: 1s
`)
	reg := newMockRegistry("test-suite")
	resolver := runner.NewScenarioResolver(reg)

	var stdout bytes.Buffer
	res, err := resolver.Resolve([]string{"--config=" + cfgFile}, &stdout)
	require.Error(t, err)
	assert.Nil(t, res)

	var notFoundErr *runner.ScenarioNotFoundError
	require.True(t, errors.As(err, &notFoundErr))
	assert.Contains(t, notFoundErr.Message, "no scenario specified")
}

func TestScenarioResolver_ScenarioNotRegisteredInCode(t *testing.T) {
	cfgFile := createTempConfig(t, `
version: "1.0"
default_scenario: unregistered
scenarios:
  unregistered:
    type: constant_vus
    vus: 1
    run_period: 1s
    vu_timeout: 1s
`)
	reg := newMockRegistry("test-suite")
	resolver := runner.NewScenarioResolver(reg)

	var stdout bytes.Buffer
	res, err := resolver.Resolve([]string{"--config=" + cfgFile}, &stdout)
	require.Error(t, err)
	assert.Nil(t, res)

	var notFoundErr *runner.ScenarioNotFoundError
	require.True(t, errors.As(err, &notFoundErr))
	assert.Equal(t, "unregistered", notFoundErr.Name)
	assert.Contains(t, notFoundErr.Message, "not registered in Suite")
}

func TestScenarioResolver_ScenarioNotInConfigFile(t *testing.T) {
	cfgFile := createTempConfig(t, `
version: "1.0"
default_scenario: defined_in_config
scenarios:
  defined_in_config:
    type: constant_vus
    vus: 1
    run_period: 1s
    vu_timeout: 1s
`)
	reg := newMockRegistry("test-suite")
	reg.Register("missing_from_config", engine.Scenario{
		RunVU: func(ctx engine.VUContext) error { return nil },
	})
	resolver := runner.NewScenarioResolver(reg)

	var stdout bytes.Buffer
	res, err := resolver.Resolve([]string{"--config=" + cfgFile, "--scenario=missing_from_config"}, &stdout)
	require.Error(t, err)
	assert.Nil(t, res)

	var notFoundErr *runner.ScenarioNotFoundError
	require.True(t, errors.As(err, &notFoundErr))
	assert.Equal(t, "missing_from_config", notFoundErr.Name)
	assert.Contains(t, notFoundErr.Message, "not defined in config file")
}

func TestScenarioResolver_SuccessFromFlagAndDefaultScenario(t *testing.T) {
	cfgFile := createTempConfig(t, `
version: "1.0"
default_scenario: scenario_default
scenarios:
  scenario_default:
    type: constant_vus
    vus: 2
    run_period: 5s
    vu_timeout: 1s
  scenario_explicit:
    type: arrival_rate
    target_tps: 10
    max_vus: 10
    run_period: 3s
    vu_timeout: 1s
`)
	reg := newMockRegistry("test-suite")
	scDefault := engine.Scenario{RunVU: func(ctx engine.VUContext) error { return nil }}
	scExplicit := engine.Scenario{RunVU: func(ctx engine.VUContext) error { return nil }}
	reg.Register("scenario_default", scDefault)
	reg.Register("scenario_explicit", scExplicit)

	resolver := runner.NewScenarioResolver(reg)

	// Test 1: Resolve from default_scenario
	var stdout1 bytes.Buffer
	res1, err := resolver.Resolve([]string{"--config=" + cfgFile}, &stdout1)
	require.NoError(t, err)
	require.NotNil(t, res1)
	assert.Equal(t, "scenario_default", res1.TargetScenario)
	assert.Equal(t, config.ScenarioTypeConstantVUs, res1.ScenarioCfg.Type)
	assert.Equal(t, 2, res1.ScenarioCfg.VUs)

	// Test 2: Resolve explicitly via --scenario flag
	var stdout2 bytes.Buffer
	res2, err := resolver.Resolve([]string{"--config=" + cfgFile, "--scenario=scenario_explicit"}, &stdout2)
	require.NoError(t, err)
	require.NotNil(t, res2)
	assert.Equal(t, "scenario_explicit", res2.TargetScenario)
	assert.Equal(t, config.ScenarioTypeArrivalRate, res2.ScenarioCfg.Type)
	assert.Equal(t, 10, res2.ScenarioCfg.TargetTPS)
}

func TestBuildSummaryData_ComprehensiveAndAlphabeticalSorting(t *testing.T) {
	store := metric.NewStore()

	// Register metrics in non-alphabetical order
	zHist := store.Duration("z_duration", nil)
	aCounter := store.Counter("a_counter", nil)
	mGauge := store.Gauge("m_gauge", nil)
	bRate := store.Rate("b_rate", nil)

	zHist.Observe(10 * time.Millisecond)
	zHist.Observe(20 * time.Millisecond)
	aCounter.Add(42)
	mGauge.Set(3.14)
	bRate.Add(5, 10)

	store.Counter(metric.MetricChecksPassed, metric.Tags{"name": "chk_pass"}).Inc()
	store.Counter(metric.MetricChecksFailed, metric.Tags{"name": "chk_fail"}).Inc()

	startTime := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	endTime := startTime.Add(5 * time.Second)

	thresholdResults := []sla.ThresholdResult{
		{
			Metric:   "z_duration",
			Stat:     "p95",
			Operator: "<",
			Target:   "50ms",
			Actual:   "20ms",
			Passed:   true,
		},
	}

	scenarioCfg := config.ScenarioConfig{
		Type:      config.ScenarioTypeConstantVUs,
		VUs:       5,
		RunPeriod: 5 * time.Second,
		VUTimeout: 1 * time.Second,
	}

	summary := runner.BuildSummaryData(runner.SummaryParams{
		SuiteName:        "order_suite",
		ScenarioName:     "place_order",
		Version:          "1.2.0",
		Commit:           "abc1234",
		StartedAt:        startTime,
		EndedAt:          endTime,
		Config:           scenarioCfg,
		MetricsStore:     store,
		ThresholdResults: thresholdResults,
		AllPassed:        true,
		Aborted:          false,
		AbortReason:      "",
	})

	assert.Equal(t, "order_suite", summary.SuiteName)
	assert.Equal(t, "place_order", summary.Scenario)
	assert.Equal(t, "1.2.0", summary.Version)
	assert.Equal(t, "abc1234", summary.Commit)
	assert.Equal(t, 5*time.Second, summary.Duration)
	assert.True(t, summary.Passed)
	assert.False(t, summary.Aborted)
	assert.Empty(t, summary.AbortReason)

	// Verify metrics are strictly sorted alphabetically: a_counter, b_rate, m_gauge, vuhive.checks.failed, vuhive.checks.passed, z_duration
	require.Len(t, summary.Metrics, 6)
	assert.Equal(t, "a_counter", summary.Metrics[0].Name)
	assert.Equal(t, "counter", summary.Metrics[0].Type)
	assert.Equal(t, int64(42), summary.Metrics[0].Count)

	assert.Equal(t, "b_rate", summary.Metrics[1].Name)
	assert.Equal(t, "rate", summary.Metrics[1].Type)
	assert.InDelta(t, 0.5, summary.Metrics[1].Rate, 0.0001)

	assert.Equal(t, "m_gauge", summary.Metrics[2].Name)
	assert.Equal(t, "gauge", summary.Metrics[2].Type)
	assert.InDelta(t, 3.14, summary.Metrics[2].Value, 0.0001)

	assert.Equal(t, metric.MetricChecksFailed, summary.Metrics[3].Name)
	assert.Equal(t, metric.MetricChecksPassed, summary.Metrics[4].Name)

	assert.Equal(t, "z_duration", summary.Metrics[5].Name)
	assert.Equal(t, "duration", summary.Metrics[5].Type)
	assert.Equal(t, int64(2), summary.Metrics[5].Count)
	assert.GreaterOrEqual(t, summary.Metrics[5].Min, 9*time.Millisecond)
	assert.GreaterOrEqual(t, summary.Metrics[5].Max, 19*time.Millisecond)

	// Verify checks
	require.Len(t, summary.Checks, 2)

	// Verify thresholds
	require.Len(t, summary.Thresholds, 1)
	assert.Equal(t, "z_duration", summary.Thresholds[0].Metric)
	assert.True(t, summary.Thresholds[0].Passed)
}

func TestBuildSummaryData_NegativeDurationClampedToZero(t *testing.T) {
	now := time.Now()
	summary := runner.BuildSummaryData(runner.SummaryParams{
		SuiteName:        "test",
		ScenarioName:     "test",
		StartedAt:        now,
		EndedAt:          now.Add(-10 * time.Second),
		MetricsStore:     nil,
		ThresholdResults: nil,
	})
	assert.Equal(t, time.Duration(0), summary.Duration)
}

func TestReportExecution_ConsoleAndJSONAndHook(t *testing.T) {
	tmpDir := t.TempDir()
	jsonOutPath := filepath.Join(tmpDir, "report.json")
	reportOutPath := filepath.Join(tmpDir, "report.txt")

	store := metric.NewStore()
	cnt := store.Counter("req_total", nil)
	cnt.Inc()

	hookInvoked := false
	var capturedSummary report.SummaryData

	hookScenario := engine.Scenario{
		RunVU: func(ctx engine.VUContext) error { return nil },
		HandleSummary: func(ctx engine.SummaryContext, summary report.SummaryData) error {
			hookInvoked = true
			capturedSummary = summary
			return nil
		},
	}

	var stdout bytes.Buffer
	logger := zerolog.Nop()

	now := time.Now()
	p := runner.ReportParams{
		SuiteName:    "suite_test",
		ScenarioName: "sc_test",
		Scenario:     hookScenario,
		ScenarioCfg: config.ScenarioConfig{
			Type:      config.ScenarioTypeConstantVUs,
			VUs:       1,
			RunPeriod: 1 * time.Second,
			VUTimeout: 1 * time.Second,
		},
		Flags: &cli.Flags{
			ReportFormat:  "console",
			ReportOut:     reportOutPath,
			JSONReportOut: jsonOutPath,
		},
		MetricsStore: store,
		ThresholdResults: []sla.ThresholdResult{
			{Metric: "req_total", Stat: "count", Operator: ">=", Target: "1", Actual: "1", Passed: true},
		},
		AllPassed:   true,
		Aborted:     false,
		AbortReason: "",
		StartedAt:   now,
		EndedAt:     now.Add(time.Second),
		Stdout:      &stdout,
		Logger:      logger,
	}

	runner.ReportExecution(context.Background(), p)

	// Verify report files were created
	assert.FileExists(t, jsonOutPath)
	assert.FileExists(t, reportOutPath)

	// Verify HandleSummary hook was invoked
	assert.True(t, hookInvoked)
	assert.Equal(t, "suite_test", capturedSummary.SuiteName)
	assert.Equal(t, "sc_test", capturedSummary.Scenario)
	assert.True(t, capturedSummary.Passed)
}

func TestReportExecution_HookErrorDoesNotPanic(t *testing.T) {
	hookScenario := engine.Scenario{
		RunVU: func(ctx engine.VUContext) error { return nil },
		HandleSummary: func(ctx engine.SummaryContext, summary report.SummaryData) error {
			return errors.New("slack webhook failed")
		},
	}

	var stdout bytes.Buffer
	logger := zerolog.Nop()
	now := time.Now()

	p := runner.ReportParams{
		SuiteName:    "suite_test",
		ScenarioName: "sc_test",
		Scenario:     hookScenario,
		ScenarioCfg:  config.ScenarioConfig{Type: config.ScenarioTypeConstantVUs},
		Flags:        &cli.Flags{ReportFormat: "console"},
		MetricsStore: metric.NewStore(),
		AllPassed:    true,
		StartedAt:    now,
		EndedAt:      now.Add(time.Second),
		Stdout:       &stdout,
		Logger:       logger,
	}

	assert.NotPanics(t, func() {
		runner.ReportExecution(context.Background(), p)
	})
}

func TestReportExecution_InvalidFilePathsHandledGracefully(t *testing.T) {
	var stdout bytes.Buffer
	logger := zerolog.Nop()
	now := time.Now()

	p := runner.ReportParams{
		SuiteName:    "suite_test",
		ScenarioName: "sc_test",
		Scenario:     engine.Scenario{RunVU: func(ctx engine.VUContext) error { return nil }},
		ScenarioCfg:  config.ScenarioConfig{Type: config.ScenarioTypeConstantVUs},
		Flags: &cli.Flags{
			ReportFormat:  "console",
			ReportOut:     "/nonexistent/directory/unwritable_report.txt",
			JSONReportOut: "/nonexistent/directory/unwritable_json.json",
		},
		MetricsStore: metric.NewStore(),
		AllPassed:    true,
		StartedAt:    now,
		EndedAt:      now.Add(time.Second),
		Stdout:       &stdout,
		Logger:       logger,
	}

	assert.NotPanics(t, func() {
		runner.ReportExecution(context.Background(), p)
	})
}


func TestRunSuite_EndToEndSuccess(t *testing.T) {
	cfgFile := createTempConfig(t, `
version: "1.0"
default_scenario: happy_path
scenarios:
  happy_path:
    type: constant_vus
    vus: 1
    run_period: 50ms
    vu_timeout: 100ms
`)
	reg := newMockRegistry("e2e-suite")
	executed := false
	reg.Register("happy_path", engine.Scenario{
		RunVU: func(ctx engine.VUContext) error {
			executed = true
			return nil
		},
	})

	var stdout bytes.Buffer
	res := runner.RunSuite(reg, []string{"--config=" + cfgFile}, &stdout)
	require.NoError(t, res.Error)
	assert.True(t, executed)
	assert.True(t, res.Passed)
	assert.False(t, res.Aborted)
}

func TestRunSuite_ThresholdFailureExits1(t *testing.T) {
	cfgFile := createTempConfig(t, `
version: "1.0"
default_scenario: failing_sla
scenarios:
  failing_sla:
    type: constant_vus
    vus: 1
    run_period: 50ms
    vu_timeout: 100ms
    thresholds:
      - metric: req_counter
        stat: count
        operator: ">="
        target: "10000000"
`)
	reg := newMockRegistry("e2e-suite")
	reg.Register("failing_sla", engine.Scenario{
		RunVU: func(ctx engine.VUContext) error {
			ctx.Metrics().Counter("req_counter", nil).Inc()
			return nil
		},
	})

	var stdout bytes.Buffer
	res := runner.RunSuite(reg, []string{"--config=" + cfgFile}, &stdout)
	require.NoError(t, res.Error)
	assert.False(t, res.Passed)
}

func TestRunSuite_SetupErrorReturnsError(t *testing.T) {
	cfgFile := createTempConfig(t, `
version: "1.0"
default_scenario: setup_fail
scenarios:
  setup_fail:
    type: constant_vus
    vus: 1
    run_period: 50ms
    vu_timeout: 100ms
`)
	reg := newMockRegistry("e2e-suite")
	reg.Register("setup_fail", engine.Scenario{
		Setup: func(ctx engine.SetupContext) (map[string]any, error) {
			return nil, errors.New("db connection refused")
		},
		RunVU: func(ctx engine.VUContext) error {
			return nil
		},
	})

	var stdout bytes.Buffer
	res := runner.RunSuite(reg, []string{"--config=" + cfgFile}, &stdout)
	require.Error(t, res.Error)

	var setupErr *engine.SetupError
	require.True(t, errors.As(res.Error, &setupErr))
	assert.Contains(t, setupErr.Error(), "db connection refused")
}

func TestRunSuite_StartupQuorumError(t *testing.T) {
	cfgFile := createTempConfig(t, `
version: "1.0"
default_scenario: quorum_fail
scenarios:
  quorum_fail:
    type: constant_vus
    vus: 5
    run_period: 500ms
    vu_timeout: 100ms
    max_pretest_retries: 0
    min_ready_ratio: 0.8
    startup_grace_period: 200ms
`)
	reg := newMockRegistry("quorum-suite")
	reg.Register("quorum_fail", engine.Scenario{
		PreTest: func(ctx engine.VUContext) error {
			if ctx.VUID() > 2 {
				return errors.New("auth failure")
			}
			return nil
		},
		RunVU: func(ctx engine.VUContext) error {
			return nil
		},
	})

	var stdout bytes.Buffer
	res := runner.RunSuite(reg, []string{"--config=" + cfgFile}, &stdout)
	require.Error(t, res.Error)
	assert.False(t, res.Passed)

	var quorumErr *engine.StartupQuorumError
	require.True(t, errors.As(res.Error, &quorumErr))
	assert.Equal(t, 5, quorumErr.Target)
	assert.Equal(t, 4, quorumErr.Required)
	assert.Equal(t, 0.8, quorumErr.Ratio)
}


