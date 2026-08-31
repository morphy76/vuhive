package vuhive

import (
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/engine"
	"github.com/morphy76/vuhive/internal/log"
	"github.com/morphy76/vuhive/internal/metric"
	"github.com/morphy76/vuhive/internal/report"
	"github.com/morphy76/vuhive/internal/runner"
)

// ExecutionResult represents the final outcome of running a test suite.
type ExecutionResult struct {
	// Passed indicates whether the scenario completed successfully and all SLA thresholds passed.
	Passed bool

	// Aborted indicates whether execution was terminated early due to abort_on_fail criteria.
	Aborted bool

	// AbortReason contains a human-readable explanation when execution is aborted early.
	AbortReason string

	// Error contains any configuration, initialization, or execution error encountered.
	Error error
}

// ExitCode returns 0 if the execution was successful and all SLA thresholds passed,
// or 1 if an error occurred, SLA thresholds breached, or execution was aborted.
func (r ExecutionResult) ExitCode() int {
	if r.Error != nil || !r.Passed || r.Aborted {
		return 1
	}
	return 0
}

// Suite is the root object that test developers interact with.
type Suite struct {
	name      string
	scenarios map[string]Scenario
	mu        sync.Mutex
	executed  atomic.Bool
}

// NewSuite creates an empty suite with the given display name.
// The name appears in terminal reports only.
func NewSuite(name string) *Suite {
	return &Suite{
		name:      name,
		scenarios: make(map[string]Scenario),
	}
}

// Name returns the suite display name.
func (s *Suite) Name() string {
	return s.name
}

// GetScenario retrieves a registered scenario by name.
func (s *Suite) GetScenario(name string) (Scenario, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sc, ok := s.scenarios[name]
	return sc, ok
}

// RegisterScenario associates a named Scenario with the suite.
// The name must exactly match a scenario key in vuhive.yaml.
// Panics if name is empty or if RunVU is nil.
// Panics if called after Execute or ExecuteWithArgs has been called.
func (s *Suite) RegisterScenario(name string, scenario Scenario) {
	if s.executed.Load() {
		panic("vuhive: cannot call RegisterScenario after Execute")
	}
	if name == "" {
		panic("vuhive: RegisterScenario called with empty name")
	}
	if scenario.RunVU == nil {
		panic("vuhive: RegisterScenario called with nil RunVU for scenario " + name)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.scenarios[name] = scenario
}

// Execute is the CLI entry point. It:
//  1. Parses CLI flags.
//  2. Loads and validates vuhive.yaml via Viper.
//  3. Resolves the target scenario (--scenario flag or default_scenario).
//  4. Executes the scenario lifecycle (Setup → ramp-up → run → ramp-down → Teardown).
//  5. Evaluates SLA thresholds.
//  6. Prints the terminal summary report and executes HandleSummary if configured.
//  7. Returns an ExecutionResult containing the execution outcome and does NOT
//     terminate the host process via os.Exit.
func (s *Suite) Execute() ExecutionResult {
	return s.ExecuteWithArgs(os.Args[1:], os.Stdout)
}

// ExecuteWithArgs executes the suite with custom CLI arguments and output writer.
func (s *Suite) ExecuteWithArgs(args []string, stdout io.Writer) ExecutionResult {
	s.executed.Store(true)
	adapter := &runnerSuiteAdapter{suite: s}
	res := runner.RunSuite(adapter, args, stdout)
	return ExecutionResult{
		Passed:      res.Passed,
		Aborted:     res.Aborted,
		AbortReason: res.AbortReason,
		Error:       translateError(res.Error),
	}
}

type publicLogEventAdapter struct {
	event log.LogEvent
}

func (e *publicLogEventAdapter) Str(key, val string) LogEvent {
	e.event.Str(key, val)
	return e
}

func (e *publicLogEventAdapter) Int(key string, val int) LogEvent {
	e.event.Int(key, val)
	return e
}

func (e *publicLogEventAdapter) Int64(key string, val int64) LogEvent {
	e.event.Int64(key, val)
	return e
}

func (e *publicLogEventAdapter) Float64(key string, val float64) LogEvent {
	e.event.Float64(key, val)
	return e
}

func (e *publicLogEventAdapter) Bool(key string, val bool) LogEvent {
	e.event.Bool(key, val)
	return e
}

func (e *publicLogEventAdapter) Dur(key string, val time.Duration) LogEvent {
	e.event.Dur(key, val)
	return e
}

func (e *publicLogEventAdapter) Err(err error) LogEvent {
	e.event.Err(err)
	return e
}

func (e *publicLogEventAdapter) Msg(msg string) {
	e.event.Msg(msg)
}

type publicLoggerAdapter struct {
	logger log.Logger
}

func (l *publicLoggerAdapter) Debug() LogEvent {
	return &publicLogEventAdapter{event: l.logger.Debug()}
}

func (l *publicLoggerAdapter) Info() LogEvent {
	return &publicLogEventAdapter{event: l.logger.Info()}
}

func (l *publicLoggerAdapter) Warn() LogEvent {
	return &publicLogEventAdapter{event: l.logger.Warn()}
}

func (l *publicLoggerAdapter) Error() LogEvent {
	return &publicLogEventAdapter{event: l.logger.Error()}
}

type publicMetricsAdapter struct {
	collector metric.Collector
}

func (m *publicMetricsAdapter) Counter(name string, tags Tags) Counter {
	return m.collector.Counter(name, metric.Tags(tags))
}

func (m *publicMetricsAdapter) Gauge(name string, tags Tags) Gauge {
	return m.collector.Gauge(name, metric.Tags(tags))
}

func (m *publicMetricsAdapter) Duration(name string, tags Tags) Duration {
	return m.collector.Duration(name, metric.Tags(tags))
}

func (m *publicMetricsAdapter) Rate(name string, tags Tags) Rate {
	return m.collector.Rate(name, metric.Tags(tags))
}

var vuContextAdapterPool = sync.Pool{
	New: func() any {
		a := &publicVUContextAdapter{}
		a.groupCallback = a.invokeGroup
		return a
	},
}

type publicSetupContextAdapter struct {
	engine.SetupContext
	loggerAdapter  publicLoggerAdapter
	metricsAdapter publicMetricsAdapter
}

func (a *publicSetupContextAdapter) Log() Logger {
	return &a.loggerAdapter
}

func (a *publicSetupContextAdapter) Metrics() MetricsCollector {
	return &a.metricsAdapter
}

func (a *publicSetupContextAdapter) Value(key any) any {
	if key == "vuhive.metrics" {
		return a.Metrics()
	}
	return a.SetupContext.Value(key)
}

func (a *publicSetupContextAdapter) HTTPConfig() HTTPConfig {
	return convertEngineHTTPConfig(a.SetupContext.HTTPConfig())
}

func (a *publicSetupContextAdapter) HTTPClients() map[string]HTTPConfig {
	return convertEngineHTTPClients(a.SetupContext.HTTPClients())
}

type publicVUContextAdapter struct {
	engine.VUContext
	loggerAdapter  publicLoggerAdapter
	metricsAdapter publicMetricsAdapter
	groupCallback  func(engCtx engine.VUContext) error
	groupFn        func(ctx VUContext) error
	childAdapter   *publicVUContextAdapter
}

func (a *publicVUContextAdapter) init(ctx engine.VUContext) {
	a.VUContext = ctx
	a.loggerAdapter.logger = ctx.Log()
	a.metricsAdapter.collector = ctx.Metrics()
}

func (a *publicVUContextAdapter) Log() Logger {
	return &a.loggerAdapter
}

func (a *publicVUContextAdapter) Metrics() MetricsCollector {
	return &a.metricsAdapter
}

func (a *publicVUContextAdapter) Value(key any) any {
	if key == "vuhive.metrics" {
		return a.Metrics()
	}
	return a.VUContext.Value(key)
}

func (a *publicVUContextAdapter) HTTPConfig() HTTPConfig {
	return convertEngineHTTPConfig(a.VUContext.HTTPConfig())
}

func (a *publicVUContextAdapter) HTTPClients() map[string]HTTPConfig {
	return convertEngineHTTPClients(a.VUContext.HTTPClients())
}

func (a *publicVUContextAdapter) Check(name string, fn CheckFunc) bool {
	if fn == nil {
		return a.VUContext.Check(name, nil)
	}
	return a.VUContext.Check(name, engine.CheckFunc(fn))
}

func (a *publicVUContextAdapter) CheckEqual(name string, actual, expected any) bool {
	return a.VUContext.CheckEqual(name, actual, expected)
}

func (a *publicVUContextAdapter) CheckTrue(name string, condition bool, failureReason ...string) bool {
	return a.VUContext.CheckTrue(name, condition, failureReason...)
}

func (a *publicVUContextAdapter) CheckNoError(name string, err error) bool {
	return a.VUContext.CheckNoError(name, err)
}

func (a *publicVUContextAdapter) invokeGroup(engCtx engine.VUContext) error {
	if a.childAdapter == nil {
		a.childAdapter = &publicVUContextAdapter{}
		a.childAdapter.groupCallback = a.childAdapter.invokeGroup
	}
	a.childAdapter.init(engCtx)
	return a.groupFn(a.childAdapter)
}

func (a *publicVUContextAdapter) Group(name string, fn func(ctx VUContext) error) error {
	if fn == nil {
		return a.VUContext.Group(name, nil)
	}
	oldFn := a.groupFn
	a.groupFn = fn
	err := a.VUContext.Group(name, a.groupCallback)
	a.groupFn = oldFn
	return err
}

type publicTeardownContextAdapter struct {
	engine.TeardownContext
	loggerAdapter  publicLoggerAdapter
	metricsAdapter publicMetricsAdapter
}

func (a *publicTeardownContextAdapter) Log() Logger {
	return &a.loggerAdapter
}

func (a *publicTeardownContextAdapter) Metrics() MetricsCollector {
	return &a.metricsAdapter
}

func (a *publicTeardownContextAdapter) Value(key any) any {
	if key == "vuhive.metrics" {
		return a.Metrics()
	}
	return a.TeardownContext.Value(key)
}

func (a *publicTeardownContextAdapter) HTTPConfig() HTTPConfig {
	return convertEngineHTTPConfig(a.TeardownContext.HTTPConfig())
}

func (a *publicTeardownContextAdapter) HTTPClients() map[string]HTTPConfig {
	return convertEngineHTTPClients(a.TeardownContext.HTTPClients())
}

type publicSummaryContextAdapter struct {
	engine.SummaryContext
	loggerAdapter  publicLoggerAdapter
	metricsAdapter publicMetricsAdapter
}

func (a *publicSummaryContextAdapter) Log() Logger {
	return &a.loggerAdapter
}

func (a *publicSummaryContextAdapter) Metrics() MetricsCollector {
	return &a.metricsAdapter
}

func (a *publicSummaryContextAdapter) Value(key any) any {
	if key == "vuhive.metrics" {
		return a.Metrics()
	}
	return a.SummaryContext.Value(key)
}

func (a *publicSummaryContextAdapter) HTTPConfig() HTTPConfig {
	return convertEngineHTTPConfig(a.SummaryContext.HTTPConfig())
}

func (a *publicSummaryContextAdapter) HTTPClients() map[string]HTTPConfig {
	return convertEngineHTTPClients(a.SummaryContext.HTTPClients())
}

func convertEngineHTTPConfig(c config.HTTPConfig) HTTPConfig {
	var headers map[string]string
	if c.Headers != nil {
		headers = make(map[string]string, len(c.Headers))
		for k, v := range c.Headers {
			headers[k] = v
		}
	}
	return HTTPConfig{
		BaseURL:        c.BaseURL,
		Timeout:        c.Timeout,
		Headers:        headers,
		TLS:            TLSConfig{InsecureSkipVerify: c.TLS.InsecureSkipVerify},
		Pool:           HTTPPoolConfig{MaxIdleConns: c.Pool.MaxIdleConns, MaxIdleConnsPerHost: c.Pool.MaxIdleConnsPerHost, IdleConnTimeout: c.Pool.IdleConnTimeout},
		DetailedTiming: c.DetailedTiming,
		MetricPrefix:   c.MetricPrefix,
	}
}

type runnerSuiteAdapter struct {
	suite *Suite
}

func convertEngineHTTPClients(clients map[string]config.HTTPConfig) map[string]HTTPConfig {
	if clients == nil {
		return nil
	}
	result := make(map[string]HTTPConfig, len(clients))
	for name, cfg := range clients {
		result[name] = convertEngineHTTPConfig(cfg)
	}
	return result
}

func (a *runnerSuiteAdapter) Name() string {
	return a.suite.name
}

func (a *runnerSuiteAdapter) GetScenario(name string) (engine.Scenario, bool) {
	sc, ok := a.suite.GetScenario(name)
	if !ok {
		return engine.Scenario{}, false
	}

	var setup engine.SetupHook
	if sc.Setup != nil {
		setup = func(ctx engine.SetupContext) (map[string]any, error) {
			adapter := &publicSetupContextAdapter{
				SetupContext:   ctx,
				loggerAdapter:  publicLoggerAdapter{logger: ctx.Log()},
				metricsAdapter: publicMetricsAdapter{collector: ctx.Metrics()},
			}
			return sc.Setup(adapter)
		}
	}

	var preTest engine.PreTestHook
	if sc.PreTest != nil {
		preTest = func(ctx engine.VUContext) error {
			adapter := vuContextAdapterPool.Get().(*publicVUContextAdapter)
			adapter.init(ctx)
			err := sc.PreTest(adapter)
			vuContextAdapterPool.Put(adapter)
			return err
		}
	}

	var runVU engine.VURunnerHook
	if sc.RunVU != nil {
		runVU = func(ctx engine.VUContext) error {
			adapter := vuContextAdapterPool.Get().(*publicVUContextAdapter)
			adapter.init(ctx)
			err := sc.RunVU(adapter)
			vuContextAdapterPool.Put(adapter)
			return err
		}
	}

	var afterTest engine.AfterTestHook
	if sc.AfterTest != nil {
		afterTest = func(ctx engine.VUContext) error {
			adapter := vuContextAdapterPool.Get().(*publicVUContextAdapter)
			adapter.init(ctx)
			err := sc.AfterTest(adapter)
			vuContextAdapterPool.Put(adapter)
			return err
		}
	}

	var teardown engine.TeardownHook
	if sc.Teardown != nil {
		teardown = func(ctx engine.TeardownContext, state map[string]any) error {
			adapter := &publicTeardownContextAdapter{
				TeardownContext: ctx,
				loggerAdapter:   publicLoggerAdapter{logger: ctx.Log()},
				metricsAdapter:  publicMetricsAdapter{collector: ctx.Metrics()},
			}
			return sc.Teardown(adapter, state)
		}
	}

	var handleSummary engine.SummaryHook
	if sc.HandleSummary != nil {
		handleSummary = func(ctx engine.SummaryContext, summary report.SummaryData) error {
			adapter := &publicSummaryContextAdapter{
				SummaryContext: ctx,
				loggerAdapter:  publicLoggerAdapter{logger: ctx.Log()},
				metricsAdapter: publicMetricsAdapter{collector: ctx.Metrics()},
			}
			return sc.HandleSummary(adapter, convertReportSummaryToPublic(summary))
		}
	}

	return engine.Scenario{
		Setup:         setup,
		PreTest:       preTest,
		RunVU:         runVU,
		AfterTest:     afterTest,
		Teardown:      teardown,
		HandleSummary: handleSummary,
	}, true
}

func convertReportSummaryToPublic(s report.SummaryData) SummaryData {
	metrics := make([]MetricSummary, len(s.Metrics))
	for i, m := range s.Metrics {
		metrics[i] = MetricSummary{
			Name:  m.Name,
			Type:  m.Type,
			Tags:  m.Tags,
			Count: m.Count,
			Value: m.Value,
			Rate:  m.Rate,
			Min:   m.Min,
			Mean:  m.Mean,
			P50:   m.P50,
			P90:   m.P90,
			P95:   m.P95,
			P99:   m.P99,
			Max:   m.Max,
		}
	}

	checks := make([]CheckSummary, len(s.Checks))
	for i, c := range s.Checks {
		checks[i] = CheckSummary{
			Name:    c.Name,
			Passed:  c.Passed,
			Failed:  c.Failed,
			Total:   c.Total,
			PassPct: c.PassPct,
		}
	}

	thresholds := make([]ThresholdSummary, len(s.Thresholds))
	for i, th := range s.Thresholds {
		thresholds[i] = ThresholdSummary{
			Metric:   th.Metric,
			Stat:     th.Stat,
			Operator: th.Operator,
			Target:   th.Target,
			Actual:   th.Actual,
			Passed:   th.Passed,
		}
	}

	groups := make([]GroupSummary, len(s.Groups))
	for i, g := range s.Groups {
		groups[i] = GroupSummary{
			Name:  g.Name,
			Count: g.Count,
			Min:   g.Min,
			Mean:  g.Mean,
			P50:   g.P50,
			P90:   g.P90,
			P95:   g.P95,
			P99:   g.P99,
			Max:   g.Max,
		}
	}

	return SummaryData{
		SuiteName:   s.SuiteName,
		Scenario:    s.Scenario,
		Version:     s.Version,
		Commit:      s.Commit,
		StartedAt:   s.StartedAt,
		EndedAt:     s.EndedAt,
		Duration:    s.Duration,
		Config:      s.Config,
		Metrics:     metrics,
		Checks:      checks,
		Groups:      groups,
		Thresholds:  thresholds,
		Passed:      s.Passed,
		Aborted:     s.Aborted,
		AbortReason: s.AbortReason,
	}
}


func translateError(err error) error {
	if err == nil {
		return nil
	}
	var cfgErr *config.ConfigError
	if errors.As(err, &cfgErr) {
		return &ConfigError{Path: cfgErr.Path, Err: cfgErr.Err}
	}
	var valErr *config.ValidationError
	if errors.As(err, &valErr) {
		return &ValidationError{Field: valErr.Field, Message: valErr.Message}
	}
	var snfErr *runner.ScenarioNotFoundError
	if errors.As(err, &snfErr) {
		return &ScenarioNotFoundError{Name: snfErr.Name, Message: snfErr.Message}
	}
	var setupErr *engine.SetupError
	if errors.As(err, &setupErr) {
		return &SetupError{Err: setupErr.Err}
	}
	return err
}

// Compile-time interface satisfaction checks.
var (
	_ LogEvent               = (*publicLogEventAdapter)(nil)
	_ Logger                 = (*publicLoggerAdapter)(nil)
	_ MetricsCollector       = (*publicMetricsAdapter)(nil)
	_ SetupContext           = (*publicSetupContextAdapter)(nil)
	_ ConfigProvider         = (*publicSetupContextAdapter)(nil)
	_ ObservabilityProvider = (*publicSetupContextAdapter)(nil)
	_ VUContext              = (*publicVUContextAdapter)(nil)
	_ ScenarioContext        = (*publicVUContextAdapter)(nil)
	_ ExecutionIdentity     = (*publicVUContextAdapter)(nil)
	_ ConfigProvider         = (*publicVUContextAdapter)(nil)
	_ StateProvider          = (*publicVUContextAdapter)(nil)
	_ ObservabilityProvider = (*publicVUContextAdapter)(nil)
	_ WorkflowController    = (*publicVUContextAdapter)(nil)
	_ TeardownContext        = (*publicTeardownContextAdapter)(nil)
	_ ConfigProvider         = (*publicTeardownContextAdapter)(nil)
	_ StateProvider          = (*publicTeardownContextAdapter)(nil)
	_ ObservabilityProvider = (*publicTeardownContextAdapter)(nil)
	_ SummaryContext         = (*publicSummaryContextAdapter)(nil)
	_ ConfigProvider         = (*publicSummaryContextAdapter)(nil)
	_ ObservabilityProvider = (*publicSummaryContextAdapter)(nil)
	_ runner.ScenarioRegistry = (*runnerSuiteAdapter)(nil)
)
