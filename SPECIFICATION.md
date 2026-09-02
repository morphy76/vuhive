# `vuhive` — Golang Load Testing Library Specification

**Module:** `github.com/morphy76/vuhive`  
**Language:** Go 1.26+  
**Revision:** 2 (post-challenge hardening)

---

## 1. Purpose & Adoption Model

`vuhive` is a **Go library** — not a standalone binary. Test developers import it, implement test logic using
lifecycle hooks, compile their own `main` package, and run the resulting binary against a YAML configuration.

```
Test Developer's Project
├── go.mod   (imports github.com/morphy76/vuhive/pkg/vuhive)
├── main.go  (registers scenarios, calls suite.Execute())
└── vuhive.yaml
```

The compiled binary is self-contained: it loads config, runs the selected scenario, emits a terminal report,
evaluates SLA thresholds, and exits with the appropriate code.

---

## 2. Module Layout

```text
github.com/morphy76/vuhive/               ← module root
├── go.mod
├── go.sum
├── VERSION.vuhive                        ← SemVer plain-text file (e.g., "0.1.0")
├── Makefile
├── SPECIFICATION.md
│
├── pkg/
│   └── vuhive/                           ← Public API surface (package vuhive)
│       ├── suite.go                     ← Suite, NewSuite(), Execute()
│       ├── scenario.go                  ← Scenario struct and all hook types
│       ├── context.go                   ← ScenarioContext interface
│       ├── metrics.go                   ← MetricsCollector, Counter, Gauge, Duration, Rate
│       ├── logger.go                    ← Logger, LogEvent interfaces
│       ├── errors.go                    ← ConfigError, ValidationError, ScenarioNotFoundError, SetupError
│       ├── check.go                     ← CheckFunc inline assertion function type
│       ├── delay.go                     ← DelayStrategy, DelayGenerator, and delay constructors
│       ├── think_time.go                ← ThinkTimeConfig
│       ├── summary.go                   ← SummaryData, MetricSummary, CheckSummary, ThresholdSummary
│       └── data/                        ← Dataset parameterization subpackage
│
├── internal/
│   ├── version/
│   │   └── version.go                   ← Version, Commit, BuildTime vars (ldflags target)
│   ├── config/                          ← YAML loading, validation, Config/ScenarioConfig types
│   ├── engine/                          ← VU lifecycle, pacing (constant_vus, arrival_rate, ramping_vus)
│   ├── metric/                          ← In-memory metrics engine (Registry, Collector, Aggregator, Reader, Store)
│   ├── sla/                             ← Threshold evaluation and results
│   ├── log/                             ← Zerolog adapter implementing vuhive.Logger
│   ├── cli/                             ← CLI flag parsing
│   ├── report/                          ← Console and JSON report formatters
│   └── runner/                          ← Execution orchestration & lifecycle dispatch
│       ├── resolver.go                  ← Scenario & config resolution
│       ├── summary.go                   ← Summary data aggregation & sorting
│       ├── reporter.go                  ← Report generation & summary hook dispatch
│       └── runner.go                    ← Suite execution coordinator
│
└── examples/
    ├── http_checkout/                   ← REST API load test (constant_vus)
    ├── grpc_user_service/               ← RPC arrival rate pacing (arrival_rate)
    ├── ramping_vus/                     ← Multi-stage spike test (ramping_vus)
    ├── conversation_flow/               ← Real-time SSE conversational AI
    ├── think_time/                      ← Thinking time & delay strategies
    ├── checks/                          ← Inline assertions (ctx.Check)
    ├── data_parameterization/           ← Dataset parameterization (CSV, JSON, JSONL)
    ├── sla_thresholds/                  ← Quality gates, multi-metrics, abort_on_fail
    └── handle_summary/                  ← Execution summary hooks (HandleSummary)
```

---

## 3. TDD Strategy & Build Tag Convention

The library is developed using **Test-Driven Development**. All framework source code is written to make a
failing test pass, followed by a refactor step.

### 3.1 Build Tag Taxonomy

| Tag | Purpose | Used in |
|-----|---------|---------|
| *(none)* | Framework unit tests — always run with `go test ./...` | `internal/**/*_test.go`, `*_test.go` at root |
| `integration` | Tests requiring external services (e.g., a real HTTP server) | `internal/**/*_integration_test.go` |
| `vuhive_example` | Compilable example load tests in `examples/` | `examples/**/*.go` |

### 3.2 Applying Tags

All `_test.go` files in the library that are internal framework tests carry **no special build tag** — they
are excluded automatically when consumers import the library (Go's module system never runs a dependency's
test files in a downstream project).

Example integration tests are guarded:
```go
//go:build vuhive_example

package main

import "github.com/morphy76/vuhive/pkg/vuhive"
```

### 3.3 Makefile Targets

All targets must be `.PHONY`.

```makefile
COMPONENT  := vuhive
VERSION    ?= $(shell cat VERSION.$(COMPONENT) 2>/dev/null || echo "0.0.0")
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
LDFLAGS    := -s -w \
  -X 'github.com/morphy76/vuhive/internal/version.Version=$(VERSION)' \
  -X 'github.com/morphy76/vuhive/internal/version.Commit=$(COMMIT)' \
  -X 'github.com/morphy76/vuhive/internal/version.BuildTime=$(BUILD_TIME)'

## test: Run all library unit tests
test:
	go test ./...

## test-integration: Run integration tests (requires external services)
test-integration:
	go test -tags=integration ./...

## test-examples: Build example binaries to verify they compile
test-examples:
	go build -tags=vuhive_example ./examples/...

## test-race: Run unit tests with race detector
test-race:
	go test -race ./...

## lint: Run static analysis
lint:
	golangci-lint run ./...

## generate: Run code generators
generate:
	go generate ./...

## help: Print this help
help:
	@grep -E '^## ' Makefile | sed 's/## //'
```

---

## 4. Public API Surface (Package `vuhive`)

This section is the **contract** between the library and test developers. Every exported symbol here must be
stable within a minor version.

### 4.1 `Logger` & `LogEvent` Interfaces

The `Logger` interface mirrors the zerolog fluent API subset without exposing the concrete zerolog type.

```go
// LogEvent is a fluent builder for a single log message.
// Callers must terminate with Msg() to emit the event.
type LogEvent interface {
    Str(key, val string) LogEvent
    Int(key string, val int) LogEvent
    Int64(key string, val int64) LogEvent
    Float64(key string, val float64) LogEvent
    Bool(key string, val bool) LogEvent
    Dur(key string, val time.Duration) LogEvent
    Err(err error) LogEvent
    Msg(msg string)
}

// Logger is the scoped logger available inside a VU execution.
// The implementation is zerolog; the interface is stable.
type Logger interface {
    Debug() LogEvent
    Info() LogEvent
    Warn() LogEvent
    Error() LogEvent
}
```

### 4.2 `MetricsCollector` Interface

```go
// Tags is an optional set of key-value labels attached to a metric observation.
// Tag keys and values must be non-empty strings. Panics on nil map — use vuhive.Tags{} for no tags.
type Tags map[string]string

// MetricsCollector is available inside ScenarioContext.
// All returned metric handles are safe for concurrent use from multiple VU goroutines.
type MetricsCollector interface {
    // Counter returns a monotonically increasing counter identified by name+tags.
    Counter(name string, tags Tags) Counter

    // Gauge returns an instantaneous value handle identified by name+tags.
    Gauge(name string, tags Tags) Gauge

    // Duration returns a latency histogram identified by name+tags.
    // Internally uses per-VU HDR histograms merged at report time.
    Duration(name string, tags Tags) Duration

    // Rate returns a ratio tracker identified by name+tags.
    // Test developers record numerator and denominator together.
    // Threshold stat "rate" computes sum(numerator)/sum(denominator) across all observations.
    Rate(name string, tags Tags) Rate
}

type Counter interface {
    Inc()
    Add(delta int64)
}

type Gauge interface {
    Set(value float64)
    Add(delta float64)
}

type Duration interface {
    // Observe records one latency sample.
    Observe(d time.Duration)
}

type Rate interface {
    // Add records `numerator` events out of `denominator` total attempts.
    // Both must be >= 0. Denominator == 0 is ignored (no observation recorded).
    Add(numerator, denominator int64)
}
```

### 4.3 `ScenarioContext` and Composed Context Interfaces

To adhere to the **Interface Segregation Principle (ISP)** and eliminate dummy/meaningless methods during non-VU lifecycle phases, the framework defines focused single-responsibility capability interfaces and aggregates them into role-specific composed context interfaces:

#### Capability Interfaces
```go
// ExecutionIdentity provides execution identity attributes (VU ID, iteration index, and scenario name).
type ExecutionIdentity interface {
    // VUID returns the 1-based unique identifier for this Virtual User goroutine.
    VUID() int64

    // Iteration returns the 0-based iteration count for the current VU.
    // Always 0 inside PreTest and AfterTest hooks.
    Iteration() int64

    // ScenarioName returns the active scenario name as declared in vuhive.yaml.
    ScenarioName() string
}

// ConfigProvider provides access to scenario configuration parameters.
type ConfigProvider interface {
    // Param retrieves a string value from the scenario's params map.
    // Returns "" if key is absent.
    Param(key string) string

    // ParamInt retrieves a params value parsed as int.
    // Returns defaultValue if key is absent. If present but unparseable, emits a Warn-level log and returns defaultValue.
    ParamInt(key string, defaultValue int) int

    // ParamDuration retrieves a params value parsed as time.Duration.
    // Returns defaultValue if key is absent. If present but unparseable, emits a Warn-level log and returns defaultValue.
    ParamDuration(key string, defaultValue time.Duration) time.Duration
}

// StateProvider provides read-only access to global scenario state returned by Setup.
type StateProvider interface {
    // GlobalState retrieves a value from the map returned by the Setup hook.
    // The map is read-only and shared across all VUs; callers must not mutate it.
    // Returns nil if key is absent or Setup was not provided.
    GlobalState(key string) any
}

// ObservabilityProvider provides access to structured logging and metric collection.
type ObservabilityProvider interface {
    // Log returns the structured logger pre-enriched with scenario/VU/iteration context.
    Log() Logger

    // Metrics returns the metrics collector handle for recording custom telemetry.
    Metrics() MetricsCollector
}

// WorkflowController provides workflow execution controls such as delays and inline assertions.
type WorkflowController interface {
    // Sleep pauses execution for explicit duration or configured interaction_delay strategy.
    Sleep(d ...time.Duration) error

    // Check evaluates an inline pass/fail assertion function without stopping iteration.
    Check(name string, fn CheckFunc) bool

    // CheckEqual evaluates actual == expected under check name with zero allocations on pass.
    CheckEqual(name string, actual, expected any) bool

    // CheckTrue evaluates condition under check name.
    CheckTrue(name string, condition bool, failureReason ...string) bool

    // CheckNoError evaluates that err == nil under check name.
    CheckNoError(name string, err error) bool

    // Group executes fn within a named transaction boundary.
    Group(name string, fn func(ctx VUContext) error) error
}
```

#### Role-Specific Composed Context Interfaces
```go
// SetupContext provides configuration access and structured observability during scenario setup.
type SetupContext interface {
    context.Context
    ConfigProvider
    ObservabilityProvider
}

// VUContext is the scoped execution context passed to active Virtual User hooks (PreTest, RunVU, AfterTest).
type VUContext interface {
    context.Context
    ExecutionIdentity
    ConfigProvider
    StateProvider
    ObservabilityProvider
    WorkflowController
}

// TeardownContext provides configuration, read-only global state, and observability for scenario teardown.
type TeardownContext interface {
    context.Context
    ConfigProvider
    StateProvider
    ObservabilityProvider
}

// SummaryContext provides context cancellation, scenario params, and structured logging for post-run reporting.
type SummaryContext interface {
    context.Context
    ConfigProvider
    ObservabilityProvider
}

// ScenarioContext is the scoped execution context passed to every VU hook.
// It embeds context.Context and composes focused capability interfaces.
// Maintained for backward compatibility as equivalent to VUContext.
type ScenarioContext interface {
    context.Context
    ExecutionIdentity
    ConfigProvider
    StateProvider
    ObservabilityProvider
    WorkflowController
}
```

> **Note on Global State Thread Safety:** The `map[string]any` returned by `Setup` is treated as **immutable**
> after Setup returns. The framework makes a shallow copy of the map and exposes it read-only via `GlobalState()`.
> **Shallow Copy Limitation:** Shallow copy only clones top-level map keys and does not deep-copy nested mutable
> structures (e.g. slices, maps, pointer structs). Test developers must ensure that any complex objects returned
> by `Setup` are strictly immutable or protected with thread-safe concurrency primitives (`sync.RWMutex`, `sync.Map`, etc.).

### 4.4 Hook Types

```go
// SetupHook is called once before any VU is spawned.
// It returns a global state map shared (read-only) with all VUs via VUContext.GlobalState().
// A non-nil error aborts the test run immediately.
type SetupHook func(ctx SetupContext) (state map[string]any, err error)

// PreTestHook is called once per VU goroutine before its iteration loop begins.
// A non-nil error skips RunVU but still guarantees AfterTestHook execution for that VU.
type PreTestHook func(ctx VUContext) error

// VURunnerHook is called repeatedly in a loop for each VU during the run_period.
// Each call receives a fresh child context with the vu_timeout deadline applied.
// A non-nil error or panic is caught, logged, and counted; the loop continues.
type VURunnerHook func(ctx VUContext) error

// AfterTestHook is called once per VU after the run loop ends (or after PreTest failure).
// It runs in a deferred call, so it executes even if RunVU panicked.
// A non-nil error is logged but does not affect the overall pass/fail verdict.
type AfterTestHook func(ctx VUContext) error

// TeardownHook is called once after all VU goroutines have exited.
// It receives the same global state produced by Setup.
// A non-nil error is logged but does not affect the overall pass/fail verdict.
type TeardownHook func(ctx TeardownContext, state map[string]any) error

// SummaryHook is called after test execution and report generation.
// It receives the execution context and complete execution summary data.
type SummaryHook func(ctx SummaryContext, summary SummaryData) error
```

### 4.5 `Scenario` Struct

```go
// Scenario groups all lifecycle hooks for a named test scenario.
// Only RunVU is required. All other hooks are optional and may be nil.
type Scenario struct {
    Setup         SetupHook     // optional
    PreTest       PreTestHook   // optional
    RunVU         VURunnerHook  // required
    AfterTest     AfterTestHook // optional
    Teardown      TeardownHook  // optional
    HandleSummary SummaryHook   // optional
}
```

**Validation:** `RegisterScenario` panics if `RunVU` is nil.

### 4.6 `Suite` API

```go
// Suite is the root object that test developers interact with.
type Suite struct { /* unexported fields */ }

// NewSuite creates an empty suite with the given display name.
// The name appears in terminal reports only.
func NewSuite(name string) *Suite

// RegisterScenario associates a named Scenario with the suite.
// The name must exactly match a scenario key in vuhive.yaml.
// Panics if name is empty or if RunVU is nil.
// Panics if called after Execute has been called.
func (s *Suite) RegisterScenario(name string, scenario Scenario)

// ExecutionResult represents the final outcome of running a test suite.
type ExecutionResult struct {
	Passed      bool
	Aborted     bool
	AbortReason string
	Error       error
}

// ExitCode returns 0 if all thresholds passed and no error/abort occurred, or 1 otherwise.
func (r ExecutionResult) ExitCode() int

// Execute is the CLI entry point. It:
//  1. Parses CLI flags (see §6 for the full flag inventory).
//  2. Loads and validates vuhive.yaml via Viper.
//  3. Resolves the target scenario (--scenario flag or default_scenario).
//  4. Executes the scenario lifecycle (Setup → ramp-up → run → ramp-down → Teardown).
//  5. Evaluates SLA thresholds.
//  6. Prints the terminal summary report and executes HandleSummary if configured.
//  7. Returns an ExecutionResult containing the execution outcome and does NOT
//     terminate the host process via os.Exit.
func (s *Suite) Execute() ExecutionResult
```

**Error Taxonomy for `Execute()`:**

| Condition | Behavior |
|-----------|----------|
| `--config` file not found | Returns `ExecutionResult{Error: *vuhive.ConfigError}` |
| YAML parse error | Returns `ExecutionResult{Error: *vuhive.ConfigError}` |
| Validation invariant violated | Returns `ExecutionResult{Error: *vuhive.ValidationError}` |
| Scenario in `--scenario` not registered | Returns `ExecutionResult{Error: *vuhive.ScenarioNotFoundError}` |
| Scenario registered but not in config | Returns `ExecutionResult{Error: *vuhive.ScenarioNotFoundError}` |
| `Setup` hook returns error | Returns `ExecutionResult{Error: *vuhive.SetupError}` wrapping the hook error |
| Startup quorum health gate breached | Returns `ExecutionResult{Error: *vuhive.StartupQuorumError}`, `ExitCode() == 1` |
| SLA threshold breached | Returns `ExecutionResult{Passed: false}`, `ExitCode() == 1` |
| Execution aborted via abort_on_fail | Returns `ExecutionResult{Passed: false, Aborted: true, AbortReason: "..."}`, `ExitCode() == 1` |
| Clean completion, all thresholds pass | Returns `ExecutionResult{Passed: true}`, `ExitCode() == 0` |

---

## 5. VU Lifecycle — Detailed State Machine

### 5.1 Execution Order Guarantee

```text
[Suite.Execute() called]
        │
        ▼
 ┌─ Load & Validate Config ──────────────────┐
 │  fail → return ConfigError/ValidationError│
 └───────────────────────────────────────────┘
        │
        ▼
 ┌─ Setup (once, sequential) ────────────────┐
 │  fail → return SetupError                 │
 │  success → globalState map captured       │
 └───────────────────────────────────────────┘
        │
        ▼  [spawn VU goroutines per pacing schedule]
        │
   per VU goroutine:
   ┌──────────────────────────────────────────────────────────────────────┐
   │  defer AfterTest()   ← registered BEFORE PreTest is called          │
   │                                                                      │
   │  PreTest() ──fail──► log error, count vu_pretest_errors, skip RunVU │
   │      │                                                               │
   │  (success)                                                           │
   │      │                                                               │
   │  loop while run_period not expired AND ctx not cancelled:            │
   │      │                                                               │
   │    iteration ctx = context.WithTimeout(ctx, vu_timeout)             │
   │    RunVU(iterationCtx)                                               │
   │      ├─ panic ──► recover, log+count error_panic, continue loop     │
   │      ├─ error ──► log+count vu_iteration_errors, continue loop      │
   │      └─ nil   ──► count vu_iterations_total, continue loop          │
   │                                                                      │
   │  [deferred AfterTest() runs here regardless of what happened above]  │
   └──────────────────────────────────────────────────────────────────────┘
        │
        ▼  [all VU goroutines exited]
        │
 ┌─ Teardown (once, sequential) ─────────────┐
 │  error logged, does not affect verdict    │
 └───────────────────────────────────────────┘
        │
        ▼
 ┌─ SLA Threshold Evaluation ─────────────────┐
 │  breach → print report, Passed=false       │
 │  pass   → print report, Passed=true        │
 └────────────────────────────────────────────┘
```

### 5.2 `vu_timeout` Scope

`vu_timeout` applies **per `RunVU` iteration**. For each call to `RunVU`, the framework creates a child
context with `context.WithTimeout(parentCtx, vuTimeout)`. A deadline exceeded counts as a failed iteration
and increments the built-in `vu_iteration_errors` counter. The run loop continues with the next iteration.

### 5.3 Built-In Framework Metrics

The framework automatically records these metrics (always available in the report, not controllable by
test developers):

| Metric Constant | String Identifier | Type | Description |
|-----------------|-------------------|------|-------------|
| `MetricIterationsTotal` | `vuhive.vu.iterations_total` | Counter | Total completed `RunVU` calls (success + failure) |
| `MetricIterationsFailed` | `vuhive.vu.iterations_failed` | Counter | `RunVU` calls returning non-nil error or panic |
| `MetricIterationsTimeout` | `vuhive.vu.iterations_timeout` | Counter | `RunVU` calls that hit the `vu_timeout` deadline |
| `MetricVUPanics` | `vuhive.vu.panics` | Counter | `RunVU` panics recovered by the framework |
| `MetricVUPretestErrors` | `vuhive.vu.pretest_errors` | Counter | VUs where `PreTest` returned non-nil error |
| `MetricVUActive` | `vuhive.vu.active` | Gauge | Active VU goroutines at current instant |
| `MetricIterationDuration` | `vuhive.vu.iteration_duration` | Duration | Latency of completed VU iterations |
| `MetricPacingDroppedIterations` | `vuhive.pacing.dropped_iterations` | Counter | Arrival-rate tokens dropped due to pool saturation |
| `MetricChecksPassed` | `vuhive.checks.passed` | Counter | Total inline checks that passed |
| `MetricChecksFailed` | `vuhive.checks.failed` | Counter | Total inline checks that failed |

Built-in metric names are defined as exported package-level constants directly in `pkg/vuhive/metrics.go` (and mirrored in `internal/metric/names.go` for internal telemetry). All framework metrics are prefixed with `MetricPrefix = "vuhive."` and must not be used as names for test-developer-defined custom metrics.

> **Note on In-Flight Iterations:** `MetricIterationsTimeout` only counts iterations that exceed `vu_timeout` during active scenario execution. In-flight iterations interrupted mid-flight by scenario completion (e.g. `run_period` / `ramp_down` expiration or early scenario abort/cancellation) are discarded as incomplete and are not recorded as timeouts or failed iterations.

---

## 6. CLI Flag Inventory

`suite.Execute()` registers and parses the following flags using the `flag` standard library package
(no third-party CLI framework required).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `vuhive.yaml` | Path to the YAML configuration file |
| `--scenario` | string | value of `default_scenario` in YAML | Name of the scenario to execute |
| `--log-level` | string | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `--log-format` | string | `pretty` | Log output format: `pretty` (human-readable) or `json` |
| `--report-format` | string | `console` | Report format: `console` or `json` |
| `--report-out` | string | *(stdout)* | Write final report to this file path instead of stdout |
| `--json-report-out` | string | *(disabled)* | Write an additional JSON report document to this file path. Console report is always printed to stdout regardless of this flag |
| `--version` | bool | false | Print library version and exit |

---

## 7. Configuration Reference

### 7.1 Complete Field Table

| Field | Type | Required | Default | Validation |
|-------|------|----------|---------|------------|
| `version` | string | yes | — | Must be `"1.0"` |
| `default_scenario` | string | no | — | Must match a key in `scenarios` if present |
| `scenarios.<name>.type` | string | yes | — | `"constant_vus"`, `"arrival_rate"`, or `"ramping_vus"` |
| `scenarios.<name>.vus` | int | if `constant_vus` | — | > 0 |
| `scenarios.<name>.target_tps` | int | if `arrival_rate` | — | > 0 |
| `scenarios.<name>.max_vus` | int | if `arrival_rate` | — | > 0, ≥ 1 |
| `scenarios.<name>.stages` | list of stage objects | if `ramping_vus` | — | ≥ 1 stage (each with `target` ≥ 0 and `duration` > 0) |
| `scenarios.<name>.ramp_up` | duration string | no | `"0s"` | ≥ 0 |
| `scenarios.<name>.run_period` | duration string | if `constant_vus`/`arrival_rate` | — | > 0 |
| `scenarios.<name>.ramp_down` | duration string | no | `"0s"` | ≥ 0 |
| `scenarios.<name>.drain` | duration string | no | `"0s"` | Grace period for in-flight VUs to complete after active dispatch (alias: `drain_period`); ≥ 0 |
| `scenarios.<name>.vu_timeout` | duration string | yes | — | > 0 |
| `scenarios.<name>.params` | map[string]string | no | `{}` | Keys and values must be non-empty strings |
| `scenarios.<name>.interaction_delay` | object | no | — | Thinking time strategy explicitly invoked via `ctx.Sleep()` (see §11, Increment 1.12) |
| `scenarios.<name>.think_time` | object | no | — | Inter-iteration pacing delay executed by engine loop |
| `scenarios.<name>.thresholds` | list | no | `[]` | See §7.2 |





Duration strings follow Go's `time.ParseDuration` format (e.g., `"15s"`, `"2m30s"`, `"500ms"`).

### 7.2 Threshold Configuration

Each threshold entry has these fields:

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `metric` | string | yes | — | Exact metric name as recorded by the test developer |
| `stat` | string | yes | — | Statistic to evaluate (see table below) |
| `operator` | string | yes | — | Comparison operator: `<`, `<=`, `>`, `>=` |
| `target` | string | yes | — | Threshold value; parsed as duration if stat is a percentile/mean/max, as float64 otherwise |
| `on_no_data` | string | no | `zero` (for `count`/`value`), `fail` (for duration/`rate`) | Missing data handling strategy (`zero`, `fail`, `pass`, `ignore`, `skip`) |
| `abort_on_fail` | bool | no | `false` | Early test termination on threshold breach |
| `delay_abort_eval` | duration | no | `0s` | Warm-up grace period before abort evaluation begins |

**Supported `stat` values:**

| `stat` value | Applies to metric type | Computed as |
|---|---|---|
| `p50` | Duration | 50th percentile of all observed durations |
| `p90` | Duration | 90th percentile |
| `p95` | Duration | 95th percentile |
| `p99` | Duration | 99th percentile |
| `mean` | Duration | Arithmetic mean of all observed durations |
| `max` | Duration | Maximum observed duration |
| `count` | Counter | Total accumulated value |
| `rate` | Rate | `sum(numerator) / sum(denominator)` across all `Rate.Add()` calls |
| `value` | Gauge | Last recorded gauge value |

**Missing Data (`on_no_data`) Strategies:**

| Strategy | Behavior on Missing / Unrecorded Metric Data |
|---|---|
| `zero` | Treats missing or unrecorded metric data as `0` (or `0s` for durations). Evaluates the comparison operator against target (e.g. `count <= 0` passes with actual `0` when no errors or events are recorded). |
| `fail` | Treats missing data as an SLA failure with `actual: no data` and `reason: no data` (natural default for duration percentiles and rate metrics). |
| `pass` | Treats missing data as passing with `actual: no data`. |
| `ignore` / `skip` | Excludes/ignores the threshold when no data is recorded with `actual: no data`. |

**Disambiguation Rule:** If `stat` is one of `{p50, p90, p95, p99, mean, max}`, `target` is parsed as
`time.Duration`. Otherwise `target` is parsed as `float64`. Configuration loading fails with a
`ValidationError` if parsing fails.

**Tag Aggregation:** Thresholds evaluate the metric aggregated **across all tag combinations**. Tag-specific
threshold evaluation is not supported in Phase 1.

### 7.3 Annotated `vuhive.yaml` Example

```yaml
version: "1.0"
default_scenario: "http_checkout_flow"

scenarios:
  http_checkout_flow:
    type: "constant_vus"
    vus: 100
    ramp_up: "15s"
    run_period: "2m"
    ramp_down: "10s"
    vu_timeout: "5s"
    params:
      base_url: "https://api.staging.example.com"
      checkout_endpoint: "/v1/checkout"
      max_retries: "3"
    thresholds:
      - metric: "http_request_duration"
        stat: "p95"
        operator: "<"
        target: "200ms"
      - metric: "checkout_success_rate"
        stat: "rate"
        operator: ">"
        target: "0.995"  # Succeed if success rate > 99.5%

  payment_throughput:
    type: "arrival_rate"
    target_tps: 500
    max_vus: 200          # Hard cap on concurrent goroutines; if the pool is saturated
                          # and the target TPS cannot be maintained, a built-in
                          # 'vuhive.pacing.dropped_iterations' counter is incremented.
    ramp_up: "10s"
    run_period: "1m"
    ramp_down: "5s"
    vu_timeout: "2s"
    params:
      gateway_url: "https://payments.staging.example.com"
    thresholds:
      - metric: "payment_duration"
        stat: "p99"
        operator: "<"
```

### 7.4 JSON Schema & IDE Autocompletion

`vuhive` publishes an official JSON Schema (`schemas/vuhive.schema.json`) supporting Draft 2020-12 / Draft-07 for IntelliSense, live validation, and documentation tooltips across VS Code, GoLand, Cursor, and Neovim.

#### In-File Schema Directive
```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/morphy76/vuhive/main/schemas/vuhive.schema.json
version: "1.0"
default_scenario: payment_processing
...
```

---

## 8. Pacing Engine Algorithms

### 8.1 Ramp-Up Algorithm (Both Modes)

Ramp-up uses **linear interpolation** of the target level over `ramp_up` duration.

- **`constant_vus`:** Target active VU count increases linearly from 0 to `vus`. The scheduler spawns
  new VU goroutines at equal time intervals: `interval = ramp_up / vus`. One goroutine is spawned every
  `interval`. During steady state, all `vus` goroutines are live.

- **`arrival_rate`:** The token bucket rate increases linearly from 0 to `target_tps` over `ramp_up`.
  The rate is updated every 100ms using `rate.NewLimiter` with the interpolated value.

### 8.2 Ramp-Down Algorithm

- **`constant_vus`:** When `ramp_up + run_period` elapses, VUs stop starting new iterations. In-flight iterations are allowed up to `ramp_down` duration to complete gracefully. When `ramp_down` expires (or immediately if `ramp_down` is `0s`), active dispatch ends and the engine enters the `drain` phase.

- **`arrival_rate`:** Token dispatch ends at `ramp_up + run_period + ramp_down`. The engine enters the `drain` phase to allow active worker pool iterations to finish.

- **`ramping_vus`:** When all configured stages complete and `ramp_down` elapses, workers stop starting new iterations and the engine enters the `drain` phase.

### 8.3 Drain Execution Phase (`drain` / `drain_period`)

The `drain` phase coordinates graceful termination of in-flight worker goroutines before scenario teardown:

- **Sequencing:** `Setup` → `ramp_up` → `run_period` → `ramp_down` → `drain` → `Teardown`.
- **Fast / Early Exit:** The engine waits on active workers up to `drain` duration. If all workers complete before `drain` expires, the engine returns immediately without unnecessary delay.
- **Safety Timeout & Diagnostic Warning:** If workers exceed `drain` duration, the engine cancels worker contexts to interrupt hung executions, waits for them to exit, and logs a structured warning (`phase=drain`, `Msg: "drain phase timed out with active workers remaining"`).
- **Discarded Interrupted Iterations:** Incomplete iterations interrupted by safety timeout cancellation are discarded without being recorded as timeouts or iteration failures.

### 8.4 Arrival Rate — Pool Saturation

If the worker pool has reached `max_vus` and a new token is available, the framework increments
`vuhive.pacing.dropped_iterations` and discards the token (does not block or back-pressure). This is
intentional: arrival rate is an open model and cannot block the arrival clock.

### 8.5 Pacing Goroutine Safety & Worker Pooling


- All VU goroutines are tracked in a `sync.WaitGroup`. `Execute()` blocks on `wg.Wait()` before proceeding
  to Teardown.
- **Worker Pool Model:** For `arrival_rate`, the engine maintains a pre-allocated worker pool of size `max_vus` consuming dispatched iteration jobs from a bounded channel, eliminating steady-state goroutine creation overhead. For `constant_vus`, `vus` persistent goroutines are spawned.
- **Zero-Allocation VU Loop:** Virtual User contexts (`ScenarioContext`) are allocated once per worker goroutine and reused across iterations, updating only iteration identity and active context in place.
- **Go Runtime Tuning:** For high-throughput load generation, operators should set `GOMEMLIMIT` (80–90% of memory limit) and elevated `GOGC` (200–500) to minimize runtime GC pauses during scenario execution.

---

## 9. In-Memory Metrics Engine

### 9.1 Implementation Strategy

| Metric Type | Implementation | Thread Safety |
|-------------|----------------|---------------|
| Counter | `atomic.Int64` | Lock-free |
| Gauge | `atomic.Uint64` (float64 bits) | Lock-free |
| Duration | 16-stripe sharded HDR Histograms & per-VU histograms; merged at report time | Contention-free striped write path |
| Rate | Two `atomic.Int64` values (numerator accumulator, denominator accumulator) | Lock-free |
| Metric Lookup | Copy-on-Write `atomic.Pointer[map[metricKey]V]` | 100% Lock-free reads |

**HDR Histogram Library:** `github.com/HdrHistogram/hdrhistogram-go` with a default range of 1µs to 60s
and 3 significant digits of precision. Durations are striped across 16 mutex-guarded shards (or dedicated per-VU instances), eliminating global mutex contention. At report time, histograms
are merged using HDR's native `Merge()` function and percentiles extracted from the merged result.

### 9.2 Metric Registration & Identity

A metric is uniquely identified by its `(name, type, sorted-tags)` triple. Two `Duration` calls with the
same name but different tags produce separate histogram instances. At report time, all instances sharing the
same name are merged (tags dropped) for threshold evaluation.

**Identity collision rule:** Calling `Counter("foo", ...)` and `Duration("foo", ...)` with the same name
causes a panic at the first conflicting registration. Metric types cannot be mixed for the same name.

---

## 10. Reporting

### 10.1 Console Summary (default `--report-format=console`)

Printed to stdout (or `--report-out` file) after ramp-down completes. Produced by the `Reporter` port
implementation.

```text
================================================================================
                        VUHIVE LOAD TEST SUMMARY
================================================================================
Scenario:     http_checkout_flow              Version: 0.1.0
Mode:         constant_vus (100 VUs)          Commit:  a1b2c3d
Duration:     00:02:25  (ramp-up: 15s | run: 2m | ramp-down: 10s)
Iterations:   14,520 total  |  12 failed (0.08%)  |  3 timeout

BUILT-IN METRICS
────────────────────────────────────────────────────────────────
vuhive.vu.iterations_total      Counter    14,520
vuhive.vu.iterations_failed     Counter    12
vuhive.vu.iterations_timeout    Counter    3
vuhive.vu.panics                Counter    0
vuhive.vu.pretest_errors        Counter    0

CUSTOM METRICS
────────────────────────────────────────────────────────────────
Metric                         Type       Count    Min     Mean    p95     p99     Max
http_request_duration          Duration   14,520   12ms    45ms    110ms   230ms   850ms
http_requests_total            Counter    14,520
http_requests_failed           Counter    12
checkout_success_rate          Rate       14,520   (rate: 0.9992)

SLA THRESHOLD EVALUATION
────────────────────────────────────────────────────────────────
  [PASS]  http_request_duration   p95 < 200ms     → actual: 110ms
  [PASS]  checkout_success_rate   rate > 0.995    → actual: 0.9992
────────────────────────────────────────────────────────────────
OVERALL: PASSED                                          (exit 0)
================================================================================
```

### 10.2 JSON Report (`--report-format=json`)

Emits a structured JSON document to stdout or `--report-out`. Schema:

```json
{
  "suite_name": "E-Commerce Load Tests",
  "scenario": "http_checkout_flow",
  "version": "0.1.0",
  "commit": "a1b2c3d",
  "started_at": "2026-08-13T17:30:00Z",
  "ended_at": "2026-08-13T17:32:25Z",
  "config": { ... },
  "metrics": [
    {
      "name": "http_request_duration",
      "type": "duration",
      "tags": {},
      "count": 14520,
      "min_ms": 12,
      "mean_ms": 45,
      "p50_ms": 40,
      "p90_ms": 95,
      "p95_ms": 110,
      "p99_ms": 230,
      "max_ms": 850
    }
  ],
  "thresholds": [
    {
      "metric": "http_request_duration",
      "stat": "p95",
      "operator": "<",
      "target": "200ms",
      "actual": "110ms",
      "passed": true
    }
  ],
  "passed": true
}
```

---

## 11. Increment Deliverables with TDD Structure

Each increment specifies: what to build, the acceptance criteria, and the TDD red/green cycle guide.

---

### Increment 1.1 — Core Domain Model & Lifecycle Contracts ✅ COMPLETED (2026-08-13)

**Goal:** Define the `Scenario`, `Suite`, `ScenarioContext`, hook types, and `Logger`/`LogEvent` interfaces.
No execution engine yet — just types and compilation guarantees.

**Deliverables:**
- `pkg/vuhive/logger.go` — `Logger`, `LogEvent` interfaces
- `pkg/vuhive/metrics.go` — `MetricsCollector`, `Counter`, `Gauge`, `Duration`, `Rate` interfaces + `Tags` type
- `pkg/vuhive/context.go` — `ScenarioContext` interface
- `pkg/vuhive/scenario.go` — `Scenario` struct, all hook types
- `pkg/vuhive/suite.go` — `Suite` struct, `NewSuite()`, `RegisterScenario()`
- `internal/version/version.go` — version vars

**Acceptance Criteria (testable):**

```go
// AC-1.1.1: NewSuite returns a non-nil *Suite
suite := vuhive.NewSuite("test")
assert.NotNil(t, suite)

// AC-1.1.2: RegisterScenario with a nil RunVU panics
assert.Panics(t, func() {
    suite.RegisterScenario("bad", vuhive.Scenario{RunVU: nil})
})

// AC-1.1.3: RegisterScenario with an empty name panics
assert.Panics(t, func() {
    suite.RegisterScenario("", vuhive.Scenario{RunVU: func(ctx vuhive.ScenarioContext) error { return nil }})
})

// AC-1.1.4: ScenarioContext is embeddable as context.Context (compile-time check)
var _ context.Context = (vuhive.ScenarioContext)(nil)
```

**TDD Cycle:** Write all AC tests first. Compile will fail. Add interface definitions and stub types to make
compile succeed with tests still failing. Implement `NewSuite` and `RegisterScenario` to make tests pass.

---

### Increment 1.2 — Configuration Loading & Validation ✅ COMPLETED (2026-08-13)

**Goal:** Load and validate `vuhive.yaml` via Viper. Produce typed `Config` and `ScenarioConfig` structs.
Return structured errors for all failure modes.

**Deliverables:**
- `internal/config/loader.go` — Viper-based config loading
- `internal/config/dto.go` — `configDTO`, `scenarioConfigDTO`, `thinkTimeConfigDTO`, `thresholdConfigDTO` parsing DTOs with mapstructure tags and mapping methods
- `internal/config/model.go` — `Config`, `ScenarioConfig`, `ThinkTimeConfig`, `ThresholdConfig` types (pure domain models without framework tags)
- `internal/config/errors.go` — Structured error types (`ConfigError`, `ValidationError`)
- `internal/config/validate.go` — Validation logic, stat/operator lookup tables, and delay validator registry


**Acceptance Criteria (testable):**

```go
// AC-1.2.1: Valid YAML round-trips correctly
// AC-1.2.2: Missing required field returns ValidationError
// AC-1.2.3: Unknown scenario in default_scenario returns ValidationError
// AC-1.2.4: arrival_rate without target_tps returns ValidationError
// AC-1.2.5: constant_vus without vus returns ValidationError
// AC-1.2.6: threshold with invalid operator returns ValidationError
// AC-1.2.7: threshold target "200ms" parses correctly for Duration stats
// AC-1.2.8: threshold target "0.005" parses correctly for rate stat
// AC-1.2.9: threshold target "200ms" for "rate" stat returns ValidationError
```

**Test Data:** Use embedded YAML strings via `strings.NewReader`; no file I/O needed in unit tests.

---

### Increment 1.3 — In-Memory Metrics Engine ✅ COMPLETED (2026-08-13)

**Goal:** Implement `MetricsCollector` with all four metric types. Verify thread safety.

**Deliverables:**
- `internal/metric/names.go` — Built-in telemetry metric name constants
- `internal/metric/registry.go` — `Registry` interface & thread-safe type registry
- `internal/metric/collector.go` — `Collector` interface & metric ingestion with generic double-checked locking sync helper
- `internal/metric/aggregator.go` — `Aggregator` interface & summary statistics computation
- `internal/metric/reader.go` — `Reader` interface composing `Registry` and `Aggregator`
- `internal/metric/store.go` — `Store` composing Registry, Collector, and Aggregator
- `internal/metric/counter.go`, `gauge.go`, `histogram.go`, `rate.go` — concrete metric types
- HDR histogram per-VU lifetime management and merge API for report time

**Acceptance Criteria:**

```go
// AC-1.3.1: Counter.Inc is atomic — concurrent increments from 100 goroutines produce exact total
// AC-1.3.2: Counter with identical name+tags returns the same instance
// AC-1.3.3: Counter and Duration with same name panic (type collision)
// AC-1.3.4: Duration.Observe stores values retrievable as p50/p95/p99 within HDR precision
// AC-1.3.5: Rate.Add(1,1) + Rate.Add(0,1) produces rate = 0.5
// AC-1.3.6: Rate.Add(x, 0) is a no-op (denominator 0 is ignored)
// AC-1.3.7: Gauge.Set(5.0) + Gauge.Add(-2.0) produces 3.0
// AC-1.3.8: Tags {"a":"1"} and {"a":"2"} produce separate Counter instances for same name
```

**TDD Note:** Use `go test -race` as a mandatory step for this increment.

---

### Increment 1.4 — SLA Threshold Evaluator ✅ COMPLETED (2026-08-13)

**Goal:** Evaluate a set of `ThresholdConfig` entries against an `InMemoryMetricsStore` snapshot.
Return a structured result per threshold.

**Deliverables:**
- `internal/sla/evaluator.go` — threshold evaluation logic
- `internal/sla/result.go` — `ThresholdResult` type

**Acceptance Criteria:**

```go
// AC-1.4.1: p95 < 200ms passes when actual p95 = 110ms
// AC-1.4.2: p95 < 200ms fails when actual p95 = 250ms
// AC-1.4.3: rate > 0.995 passes when rate = 0.9992
// AC-1.4.4: count >= 100 passes when count = 100
// AC-1.4.5: Metric not found in store returns a failed threshold with "no data" reason
// AC-1.4.6: All thresholds evaluated even if the first one fails (no short-circuit)
```

---

### Increment 1.5 — Zerolog Logger Adapter ✅ COMPLETED (2026-08-13)

**Goal:** Implement the public `Logger` / `LogEvent` interfaces backed by `zerolog`. Ensure automatic
field injection (`scenario`, `vu_id`, `iteration`) and correct log level routing.

**Deliverables:**
- `internal/log/zerolog.go` — zerolog adapter implementing `vuhive.Logger` and `vuhive.LogEvent`

**Acceptance Criteria:**

```go
// AC-1.5.1: Log().Info().Str("k","v").Msg("m") emits one JSON line with level=info, k=v, message=m
// AC-1.5.2: Logger built with vuID=3 auto-injects vu_id=3 on every event
// AC-1.5.3: Debug events are suppressed when log level is set to "info"
// AC-1.5.4: Logger satisfies the vuhive.Logger interface (compile-time check)
```

**Test Strategy:** Use `zerolog.New(buf)` with a `bytes.Buffer` and parse emitted JSON lines.

---

### Increment 1.6 — Constant VU Pacing Engine ✅ COMPLETED (2026-08-13)

**Goal:** Implement the `constant_vus` execution engine: VU spawning, ramp-up, steady state,
ramp-down, and lifecycle orchestration. Exercise all lifecycle hooks.

**Deliverables:**
- `internal/engine/pacer.go` — `PacingEngine` interface, `ConstantVUsPacer`, `ArrivalRatePacer`, and `PacingRegistry`
- `internal/engine/constant_vus.go` — constant VU pacing and ramp-up/ramp-down
- `internal/engine/executor.go` — scenario lifecycle orchestration (Setup → VUs → Teardown)
- `internal/engine/scenario.go` — scenario definition, lifecycle hook types, and error wrappers
- `internal/engine/context.go` — ScenarioContext capability interfaces and context implementation

**Acceptance Criteria:**

```go
// AC-1.6.1: With vus=3, ramp_up=0, run_period=100ms, exactly 3 VU goroutines are active during run
// AC-1.6.2: Setup is called exactly once before any PreTest
// AC-1.6.3: PreTest is called exactly once per VU
// AC-1.6.4: RunVU is called at least once per VU during a 100ms run_period
// AC-1.6.5: AfterTest is called exactly once per VU, even when RunVU returns an error
// AC-1.6.6: Teardown is called exactly once after all VUs exit
// AC-1.6.7: A RunVU panic does not terminate other VUs
// AC-1.6.8: A PreTest failure skips RunVU but still calls AfterTest for that VU
// AC-1.6.9: vu_timeout causes a context.DeadlineExceeded which counts as a failed iteration;
//            the loop does not exit — RunVU is called again on the next iteration
// AC-1.6.10: ramp_up=200ms, vus=4 → VU goroutines spawned at ~50ms intervals
```

**Test Strategy:** Use hook functions with `sync.Mutex`-protected call counters and channels. Keep
`run_period` short (50ms–200ms) in tests. Use `require.Eventually` with 1s timeout for goroutine
completion assertions.

---

### Increment 1.7 — Arrival Rate Pacing Engine ✅ COMPLETED (2026-08-13)

**Goal:** Implement the `arrival_rate` execution engine with token bucket dispatch and `max_vus` pool cap.

**Deliverables:**
- `internal/engine/arrival_rate.go` — arrival rate pacing with token bucket and `max_vus` pool
- `vuhive.pacing.dropped_iterations` counter integration

**Acceptance Criteria:**

```go
// AC-1.7.1: target_tps=10, run_period=1s → approximately 10 RunVU calls (±20% tolerance)
// AC-1.7.2: max_vus=2 with slow RunVU (sleeps 500ms) and target_tps=100 → pool saturates;
//            vuhive.pacing.dropped_iterations > 0
// AC-1.7.3: ramp_up=200ms, target_tps=10 → first iteration starts after ~100ms (midpoint of ramp)
// AC-1.7.4: All other lifecycle guarantees from Increment 1.6 apply equally to arrival_rate mode
```

---

### Increment 1.8 — CLI Adapter & Suite.Execute() ✅ COMPLETED (2026-08-13)

**Goal:** Wire config loading, pacing engine selection, SLA evaluation, and reporting behind
the `Execute()` entry point. Implement all CLI flags from §6.

**Deliverables:**
- `internal/cli/flags.go` — CLI flag parsing and wiring
- `pkg/vuhive/suite.go` — `Execute()` implementation

**Acceptance Criteria:**

```go
// AC-1.8.1: --version flag prints version string and returns ExecutionResult (no os.Exit)
// AC-1.8.2: --config pointing to nonexistent file returns *vuhive.ConfigError in ExecutionResult.Error
// AC-1.8.3: --scenario not in config returns *vuhive.ScenarioNotFoundError in ExecutionResult.Error
// AC-1.8.4: Scenario registered but not in config returns *vuhive.ScenarioNotFoundError in ExecutionResult.Error
// AC-1.8.5: All thresholds pass → report printed, ExecutionResult.ExitCode() is 0
// AC-1.8.6: Any threshold fails → report printed with [FAIL] row, ExecutionResult.ExitCode() is 1
```

**Test Strategy:** Assert `ExecutionResult` and `res.ExitCode()` values returned by `Execute()` / `ExecuteWithArgs()`.


---

### Increment 1.9 — Reporting Adapters ✅ COMPLETED (2026-08-13)

**Goal:** Implement console and JSON report formatters. Ensure the report output is deterministic
(metrics sorted alphabetically by name).

**Deliverables:**
- `internal/report/model.go` — report data model and format dispatcher
- `internal/report/summary.go` — summary data models without serialization tags and query helper methods
- `internal/report/console.go` — console summary formatter
- `internal/report/json.go` — JSON report formatter

**Acceptance Criteria:**

```go
// AC-1.9.1: Console report contains scenario name, mode, duration, iteration counts
// AC-1.9.2: Console report shows [PASS]/[FAIL] for each threshold with actual vs target value
// AC-1.9.3: JSON report is valid JSON and unmarshals to the schema defined in §10.2
// AC-1.9.4: Metrics in both report formats are sorted alphabetically by name
// AC-1.9.5: --report-out writes to the specified file, not stdout
```

---

### Increment 1.10 — Developer Examples ✅ COMPLETED (2026-08-14)

**Goal:** Provide comprehensive, runnable example load tests in `examples/` covering all core framework features introduced across Phase 1 increments to serve as end-to-end reference implementations for developers.

**Deliverables:**
- `examples/http_checkout/` (`main.go`, `vuhive.yaml`) — HTTP REST API load testing with constant VU concurrency
- `examples/grpc_user_service/` (`main.go`, `vuhive.yaml`) — RPC/gRPC simulation with arrival rate token bucket pacing
- `examples/conversation_flow/` (`main.go`, `scenario.go`, `vuhive.yaml`, `dsl/`) — Real-time event-driven SSE conversational AI simulation
- `examples/think_time/` (`main.go`, `vuhive.yaml`) — Thinking time & interaction delay strategies (`interaction_delay`, `ctx.Sleep()`, `ExpoDelay`)
- `examples/checks/` (`main.go`, `vuhive.yaml`) — Inline assertions (`ctx.Check`) and check validation reporting
- `examples/data_parameterization/` (`main.go`, `vuhive.yaml`) — Dataset parameterization for CSV, JSON, and JSONL (`pkg/vuhive/data`)
- `examples/sla_thresholds/` (`main.go`, `vuhive.yaml`) — Quality gate SLA thresholds, multi-metric assertions, and error handling with graceful abort (`abort_on_fail`)
- `examples/handle_summary/` (`main.go`, `vuhive.yaml`) — Programmatic post-execution summary hooks (`HandleSummary`) for notifications and custom exports

**Build Validation:** `go build -tags=vuhive_example ./examples/...` (`make test-examples`) must succeed.

---

### Increment 1.11 — README & Documentation ✅ COMPLETED (2026-08-13)

**Goal:** Create a comprehensive `README.md` file documenting `vuhive`, load test creation, lifecycle facilities, configuration options, and execution workflows.

**Deliverables:**
- `README.md` — Complete documentation covering overview, scenario definition, lifecycle hooks, metrics, logging, configuration, and CLI execution.

**Acceptance Criteria:**

```go
// AC-1.11.1: README.md provides clear library overview, key features, and clean Hexagonal architecture structure.
// AC-1.11.2: README.md contains code examples for creating load tests, registering scenarios, and using lifecycle hooks.
// AC-1.11.3: README.md details vuhive.yaml configuration format for constant_vus and arrival_rate pacing modes, as well as SLA thresholds.
// AC-1.11.4: README.md documents CLI flags, exit code contracts (exit 0 on pass, exit 1 on SLA breach), and report output options.
```

---

### Increment 1.12 — Interaction Delay Strategies (Think Time) ✅ COMPLETED (2026-08-14)


**Goal:** Provide configurable user interaction delay (think time) strategies to simulate realistic user pauses between actions and iterations.

**Supported Strategies:**
1. **`fixed`**: Static pause duration (e.g. `500ms`).
2. **`range`**: Uniform random distribution $U(\text{min}, \text{max})$.
3. **`expo`**: Exponential distribution with specified `mean` (Poisson arrival modeling), $D = -\text{mean} \cdot \ln(U)$, with optional `min`/`max` clamping.
4. **`gaussian`**: Normal distribution $N(\mu, \sigma)$ with specified `mean` and `std_dev`, with optional `min`/`max` clamping (negative values clamped to 0 or `min`).

**Deliverables:**
- `pkg/vuhive/delay.go` — `DelayGenerator`, `DelayStrategy`, and constructor functions (`FixedDelay`, `RangeDelay`, `ExpoDelay`, `GaussianDelay`)
- `pkg/vuhive/context.go` — `ScenarioContext.Sleep(d ...time.Duration) error` method (respects `ctx.Done()`)
- `internal/config/model.go` — `InteractionDelayConfig` struct mapped under `scenarios.<name>.interaction_delay`
- `internal/config/validate.go` — validation rules for delay configs

**Configuration Schema:**

```yaml
scenarios:
  user_checkout:
    type: constant_vus
    vus: 10
    run_period: 1m
    vu_timeout: 5s
    interaction_delay:
      type: range        # "fixed" | "range" | "expo" | "gaussian"
      min: 200ms
      max: 1s
```

**Acceptance Criteria:**

```go
// AC-1.12.1: Fixed delay returns constant duration
// AC-1.12.2: Range delay returns values uniformly distributed within [min, max]
// AC-1.12.3: Expo delay generates exponential distribution with sample mean converging to target mean
// AC-1.12.4: Gaussian delay generates normally distributed durations with specified mean and std_dev
// AC-1.12.5: All delay strategies respect min/max clamping when configured
// AC-1.12.6: ctx.Sleep() halts for generated duration and aborts immediately on ctx.Done()
```

---

### Increment 1.13 — Checks (Inline Assertions) ✅ COMPLETED (2026-08-14)

**Goal:** Enable real-time inline pass/fail assertions inside `RunVU` without terminating the iteration, aggregating results for reporting and threshold gates.

**Deliverables:**
- `pkg/vuhive/check.go` — `CheckFunc` type (`type CheckFunc func() string`) and check evaluation logic
- `pkg/vuhive/context.go` — `ScenarioContext.Check(name string, fn CheckFunc) bool`
- `internal/report/console.go` & `internal/report/json.go` — CHECKS section in summary and JSON reports
- Auto-instrumentation of built-in metrics: `vuhive.checks.passed`, `vuhive.checks.failed`

**Acceptance Criteria:**

```go
// AC-1.13.1: Passing check returns true, increments vuhive.checks.passed
// AC-1.13.2: Failing check returns false, increments vuhive.checks.failed, logs reason
// AC-1.13.3: Multiple checks per iteration are aggregated independently
// AC-1.13.4: Console and JSON reports display per-check pass/fail counts and percentages
// AC-1.13.5: Thresholds can evaluate vuhive.checks.failed and vuhive.checks.passed
```

---

### Increment 1.14 — Data Parameterization Module (`pkg/vuhive/data`) ✅ COMPLETED (2026-08-14)

**Goal:** Provide a dedicated data loading and parameterization package for CSV, JSON, and JSON Lines datasets with thread-safe distribution strategies.

**Deliverables:**
- `pkg/vuhive/data/dataset.go` — `DataSet`, `LoadCSV`, `LoadJSON`, `LoadJSONL`
- `pkg/vuhive/data/strategy.go` — `Strategy` enum (`Sequential`, `Random`, `UniquePerVU`, `SharedQueue`) and thread-safe row selection

**Acceptance Criteria:**

```go
// AC-1.14.1: LoadCSV parses CSV with headers into string key-value maps
// AC-1.14.2: LoadJSON parses JSON array of objects into key-value maps
// AC-1.14.3: LoadJSONL parses newline-delimited JSON objects
// AC-1.14.4: Sequential strategy round-robins across rows deterministically by VU ID and iteration
// AC-1.14.5: Random strategy selects rows uniformly with thread safety
// AC-1.14.6: SharedQueue strategy dispenses each row exactly once across concurrent VUs
```

---

### Increment 1.15 — Execution Summary Hooks (`HandleSummary`) ✅ COMPLETED (2026-08-14)

**Goal:** Allow test developers to programmatically receive the complete structured execution summary post-run (for Slack alerts, webhooks, or custom output generation).

**Deliverables:**
- `pkg/vuhive/scenario.go` — `SummaryHook` type (`func(ctx SummaryContext, summary SummaryData) error`) and `Scenario.HandleSummary` field
- `pkg/vuhive/summary.go` — `SummaryData` export struct containing run metadata, metrics snapshot, and SLA results
- `internal/runner/reporter.go` & `internal/runner/runner.go` — post-report execution of `HandleSummary`

**Acceptance Criteria:**

```go
// AC-1.15.1: HandleSummary is invoked after test completion and report generation
// AC-1.15.2: SummaryData contains complete suite name, scenario name, execution duration, metrics, and SLA threshold results
// AC-1.15.3: Error returned by HandleSummary is logged as an error but does not mutate the exit code
```

---

### Increment 1.16 — Graceful Abort / Early Stop (`abort_on_fail`) ✅ COMPLETED (2026-08-14)

**Goal:** Support early test termination when critical thresholds breach during test execution, avoiding wasted resources or runaway system damage.

**Deliverables:**
- `internal/config/model.go` — `AbortOnFail bool` and `DelayAbortEval time.Duration` fields in `ThresholdConfig`
- `internal/config/validate.go` — validation for abort-on-fail threshold options
- `internal/engine/abort.go` — periodic background threshold monitor during run
- `internal/report/console.go` — report banner reflecting `ABORTED` verdict when early stop is triggered

**Acceptance Criteria:**

```go
// AC-1.16.1: Threshold configured with abort_on_fail=true evaluates periodically during test execution
// AC-1.16.2: Breach before delay_abort_eval is ignored (warm-up grace period)
// AC-1.16.3: Breach after delay_abort_eval cancels all active VU contexts immediately
// AC-1.16.4: Aborted test generates report showing ABORTED status and exits with code 1
```

---

## 12. Phase 2: Kubernetes Integration (Draft)

### Increment 2.1 — Docker & Helm Packaging
- Multi-stage Dockerfile producing a static binary on `distroless/static-debian12`.
- Helm chart at `deploy/helm/vuhive-operator/` with `Chart.yaml`, `values.yaml`,
  `templates/deployment.yaml`, `templates/configmap.yaml`, `templates/job-template.yaml`.

### Increment 2.2 — REST Triggering API (Gin)
```
POST   /api/v1/tests/run         → trigger; returns run ID
GET    /api/v1/tests/:id/status  → {state, started_at, ended_at}
DELETE /api/v1/tests/:id         → cancel in-flight run
GET    /api/v1/tests/:id/report  → returns JSON report (same schema as §10.2)
```

### Increment 2.3 — Background & Cron Scheduling
- Native cron expressions via K8s `CronJob`.
- Isolated `Job` pod per run; report stored in ephemeral volume or object storage.

### Increment 2.4 — K8s Observability
- Structured JSON logs (`--log-format=json`) compatible with Loki/FluentBit log aggregation.

---

## 13. Architectural Decisions Log

| # | Decision | Selected Approach | Rationale |
|---|----------|-------------------|-----------|
| 1 | Pacing Model | Both `constant_vus` and `arrival_rate` in Phase 1 | Config-driven; covers both closed and open load profiles |
| 2 | SLA Assertions | `thresholds` block in YAML; breach → `ExecutionResult.ExitCode() == 1` | CI/CD automation via exit code is the idiomatic shell contract |
| 3 | Concurrent Scenarios | Single scenario per CLI invocation | Prevents resource interference; multi-scenario reserved for Phase 2 orchestration layer |
| 4 | Metrics Export | In-memory only; reports as final summary | Metrics capture business outcomes, not infrastructure telemetry |
| 5 | Logger Exposure | Minimal `Logger`/`LogEvent` interface | Keeps zerolog as an internal detail; test developers depend on a stable abstraction |
| 6 | Rate Threshold | Dedicated `Rate` metric type; developer supplies numerator+denominator | Framework cannot infer denominator; explicit recording is unambiguous |
| 7 | VU Timeout Scope | Per-iteration deadline via `context.WithTimeout` | Timeout per call; run loop continues; aligns with HTTP client timeout semantics |
| 8 | PreTest Failure | Skip RunVU; AfterTest still runs via `defer` | AfterTest is a cleanup guarantee; PreTest failure is not a fatal scenario abort |
| 9 | Histogram Impl | Per-VU HDR histogram; merged at report time | Eliminates hot-path contention; HDR provides accurate percentiles |
| 10 | Global State Access | `ScenarioContext.GlobalState(key string) any` | Avoids Go's typed-key `context.Value` antipattern with raw strings |

---

## 14. Phase 3: Advanced Load Testing Capabilities (Draft)

Phase 3 extends `vuhive` with capabilities found in mature load testing frameworks (k6, Gatling, Locust) adapted to the Go library model.

### 14.1 Groups (Transaction Boundaries)

**Inspiration**: k6 `group()`, Gatling `exec().group()`.

Groups organize RunVU logic into named transaction boundaries with automatic duration tracking. This enables per-step latency reporting within multi-step user journeys.

#### Public API

```go
// Group executes fn within a named transaction boundary.
// Duration is automatically recorded to "vuhive.group.<groupName>.duration".
// Nested groups are allowed; names are concatenated with "::".
func (ctx ScenarioContext) Group(name string, fn func(ctx ScenarioContext) error) error
```

#### Usage

```go
RunVU: func(ctx vuhive.ScenarioContext) error {
    return ctx.Group("checkout_flow", func(ctx vuhive.ScenarioContext) error {
        // Step 1
        if err := ctx.Group("login", func(ctx vuhive.ScenarioContext) error {
            // ... login logic ...
            return nil
        }); err != nil {
            return err
        }

        // Step 2
        return ctx.Group("payment", func(ctx vuhive.ScenarioContext) error {
            // ... payment logic ...
            return nil
        })
    })
},
```

#### Automatic Metrics

| Metric | Type | Naming |
|--------|------|--------|
| Group duration | Duration | `vuhive.group.<name>.duration` |
| Nested group duration | Duration | `vuhive.group.<parent>::<child>.duration` |

---

### 14.2 HTTP Module (Built-in HTTP Client Helpers)

**Inspiration**: k6 `k6/http`, Gatling `http()`.

A convenience package providing an instrumented HTTP client that **automatically records** latency, status code counters, and error rates — eliminating boilerplate metric recording from every RunVU.

#### Package: `pkg/vuhive/http`

```go
import vuhivehttp "github.com/morphy76/vuhive/pkg/vuhive/http"

// Client wraps *http.Client with automatic metric instrumentation.
type Client struct { /* ... */ }

// Default returns a shared, instrumented HTTP client lazily initialized from declarative config.
func Default(ctx ContextProvider) *Client

// NewClient creates an instrumented HTTP client.
func NewClient(ctx SetupContext, opts ...Option) *Client

// NewClientFromConfig creates an instrumented HTTP client initialized from SetupContext's declarative HTTP config.
func NewClientFromConfig(ctx SetupContext, opts ...Option) *Client

// NewClientFromVUConfig creates an instrumented HTTP client initialized from VUContext's declarative HTTP config.
func NewClientFromVUConfig(ctx VUContext, opts ...Option) *Client

// Request methods return *Response with parsed body helpers.
func (c *Client) BaseURL() string
func (c *Client) Get(ctx context.Context, url string) (*Response, error)
func (c *Client) Post(ctx context.Context, url string, contentType string, body io.Reader) (*Response, error)
func (c *Client) Put(ctx context.Context, url string, contentType string, body io.Reader) (*Response, error)
func (c *Client) Delete(ctx context.Context, url string) (*Response, error)
func (c *Client) Do(ctx context.Context, req *http.Request) (*Response, error)

// Server-Sent Events (SSE) streaming methods.
func (c *Client) StreamSSE(ctx context.Context, rawURL string, opts ...StreamOption) (*SSEStream, error)
func (c *Client) DoStream(ctx context.Context, req *http.Request, opts ...StreamOption) (*SSEStream, error)

// Response wraps *http.Response with convenience methods.
type Response struct {
    StatusCode int
    Body       []byte
    Headers    http.Header
}

func (r *Response) JSON(v any) error     // Unmarshal body as JSON
func (r *Response) Text() string          // Body as string

// SSEEvent represents a single Server-Sent Event frame.
type SSEEvent struct {
    ID    string
    Event string
    Data  string
    Retry int
}

// SSEStream represents an active Server-Sent Events stream connection.
type SSEStream struct {
    StatusCode int
    Headers    http.Header
}

func (s *SSEStream) Next() bool
func (s *SSEStream) Event() SSEEvent
func (s *SSEStream) Err() error
func (s *SSEStream) Close() error
func (s *SSEStream) Events() <-chan SSEEvent
```

#### Declarative Schema (`vuhive.schema.json` & `vuhive.yaml`)

```yaml
scenarios:
  my_scenario:
    http:
      base_url: "https://api.example.com"
      timeout: 5s
      headers:
        Accept: "application/json"
        User-Agent: "vuhive/1.0"
      tls:
        insecure_skip_verify: false
      pool:
        max_idle_conns: 100
        max_idle_conns_per_host: 10
        idle_conn_timeout: 90s
      detailed_timing: false
      metric_prefix: "vuhive.http."
```

#### Automatic Metrics (per request & SSE streaming)

| Metric | Type | Tags |
|--------|------|------|
| `vuhive.http.req_duration` | Duration | `method`, `url`, `status` |
| `vuhive.http.req_failed` | Rate | `method`, `url`, `status` |
| `vuhive.http.reqs` | Counter | `method`, `url`, `status` |
| `vuhive.http.sse.connections_total` | Counter | `method`, `url`, `status` |
| `vuhive.http.sse.connect_duration` | Duration | `method`, `url`, `status` |
| `vuhive.http.sse.events_total` | Counter | `method`, `url`, `event_type` |
| `vuhive.http.sse.event_latency` | Duration | `method`, `url`, `event_type` |
| `vuhive.http.sse.stream_duration` | Duration | `method`, `url`, `status` |
| `vuhive.http.sse.errors_total` | Counter | `method`, `url` |
| `vuhive.http.req_receiving` | Duration | `method`, `url`, `status` |
| `vuhive.http.req_sending` | Duration | `method`, `url`, `status` |
| `vuhive.http.req_tls_handshaking` | Duration | `method`, `url`, `status` |
| `vuhive.http.req_connecting` | Duration | `method`, `url`, `status` |

#### Options

```go
vuhivehttp.NewClientFromConfig(ctx,
    vuhivehttp.WithBaseURL("https://api.example.com"),
    vuhivehttp.WithTimeout(5 * time.Second),
    vuhivehttp.WithHeader("Authorization", "Bearer "+token),
    vuhivehttp.WithTLSInsecureSkipVerify(),
    vuhivehttp.WithCustomMetricPrefix("checkout_api."),
)
```

---

### 14.3 Ramping VUs Executor

**Inspiration**: k6 `ramping-vus`, Gatling `rampUsersPerSec`.

A third pacing mode that allows defining multiple stages of VU count over time, enabling load spike simulations.

#### Configuration

```yaml
scenarios:
  spike_test:
    type: ramping_vus
    stages:
      - target: 10
        duration: 30s      # ramp from 0 → 10 VUs over 30s
      - target: 10
        duration: 1m       # hold at 10 VUs for 1 minute
      - target: 50
        duration: 10s      # spike to 50 VUs over 10s
      - target: 50
        duration: 2m       # hold spike for 2 minutes
      - target: 0
        duration: 30s      # ramp down to 0
    vu_timeout: 2s
```

#### Config Model

```go
type StageConfig struct {
    Target   int           // target VU count at end of stage
    Duration time.Duration // stage duration
}
```

#### Validation

- `stages` must have at least one entry.
- Each `target` must be `>= 0`.
- Each `duration` must be `> 0`.
- `vu_timeout` is required.

---

### 14.4 Scenarios (Multi-Scenario Concurrent Execution)

**Inspiration**: k6 `scenarios`, Gatling `setUp().protocols()`.

Allow running multiple named scenarios simultaneously within a single test binary invocation. Each scenario runs independently with its own pacing, VU pool, and lifecycle hooks. Metrics are tagged by scenario name for isolation.

#### Configuration

```yaml
version: "1.0"

scenarios:
  browse_catalog:
    type: constant_vus
    vus: 20
    run_period: 5m
    vu_timeout: 2s

  checkout_flow:
    type: arrival_rate
    target_tps: 10
    max_vus: 15
    run_period: 5m
    vu_timeout: 5s

# When --scenario is omitted and no default_scenario, run ALL scenarios concurrently
```

#### Behavior

- Each scenario gets its own `Setup` / `Teardown` lifecycle.
- Scenarios run **concurrently** with independent goroutine pools.
- The overall test ends when the **longest** scenario completes.
- Thresholds are evaluated per-scenario, not cross-scenario.
- Console report shows a section per scenario.

#### CLI

```bash
# Run all scenarios concurrently:
go run main.go --config vuhive.yaml

# Run specific scenarios (comma-separated):
go run main.go --scenario browse_catalog,checkout_flow

# Run a single scenario (existing behavior):
go run main.go --scenario browse_catalog
```

---

### 14.5 Real-Time Metrics Export

**Inspiration**: k6 `--out influxdb`, k6 `--out prometheus-rw`, Gatling live Graphite.

Allow streaming metrics to external observability systems during test execution for real-time monitoring dashboards.

#### Public API

```go
// MetricsExporter is implemented by adapters that stream metrics externally.
type MetricsExporter interface {
    // OnMetric is called for each metric observation during the test.
    OnMetric(name string, metricType string, value float64, tags Tags, timestamp time.Time)

    // Flush is called periodically and at test end.
    Flush(ctx context.Context) error

    // Close releases resources.
    Close() error
}
```

#### CLI Flag

```bash
go run main.go --out prometheus-rw=http://localhost:9090/api/v1/write
go run main.go --out influxdb=http://localhost:8086/vuhive
go run main.go --out json-stream=metrics.jsonl
```

#### Built-in Exporters

| Exporter | Flag Value | Output |
|----------|-----------|--------|
| JSON Lines | `json-stream=<path>` | One JSON object per metric per flush interval |
| Prometheus Remote Write | `prometheus-rw=<url>` | Standard Prometheus remote write protocol |
| StatsD | `statsd=<host:port>` | UDP StatsD protocol |

#### Configuration

```yaml
scenarios:
  my_test:
    # ... existing fields ...
    export:
      flush_interval: 5s   # how often to push metrics (default: 10s)
```

---

### 14.6 Tagging and Filtering in Reports

**Inspiration**: k6 tag filtering, Gatling assertions by group.

Allow filtering the console and JSON reports by tag dimensions, and support per-tag threshold evaluation.

#### Per-Tag Thresholds

```yaml
thresholds:
  - metric: http_req_duration
    stat: p95
    operator: "<"
    target: "100ms"
    tags:
      endpoint: "/api/fast"

  - metric: http_req_duration
    stat: p95
    operator: "<"
    target: "500ms"
    tags:
      endpoint: "/api/slow"
```

#### CLI Filtering

```bash
# Show only metrics with specific tags in the report:
go run main.go --tag-filter endpoint=/api/checkout
```

---

### Phase 3 Increment Delivery Plan

| Increment | Scope | Dependencies |
|-----------|-------|-------------|
| **3.1** | Groups (Transaction Boundaries) | None (pure framework addition) |
| **3.2** | HTTP Module | 3.1, Increment 1.13 (uses checks for auto-assertions) |
| **3.3** | Ramping VUs executor | None (new pacing engine) |
| **3.4** | Multi-Scenario execution | 3.3 (needs all executors) |
| **3.5** | Real-Time Metrics Export | None (new output pipeline) |
| **3.6** | Tag Filtering + Per-Tag Thresholds | None (config + SLA evaluator) |

---

### 14.7 Increment 1.13 — Checks (Inline Assertions)

Enable real-time inline pass/fail assertions inside `RunVU` without terminating the iteration, aggregating results for reporting and SLA threshold gates.

#### Public API (`pkg/vuhive`)
- `type CheckFunc func() string`: Return `""` (empty string) on pass; return non-empty failure reason on fail.
- `ScenarioContext.Check(name string, fn CheckFunc) bool`: Executes `fn()`, records metrics, logs warnings on failure, and returns boolean verdict.

#### Built-in Metrics
- `vuhive.checks.passed` (Counter): Incremented with tag `name: <check_name>` when check passes.
- `vuhive.checks.failed` (Counter): Incremented with tag `name: <check_name>` when check fails.

#### Acceptance Criteria
- **AC-1.13.1**: Passing check returns `true`, increments `vuhive.checks.passed`.
- **AC-1.13.2**: Failing check returns `false`, increments `vuhive.checks.failed`, logs reason.
- **AC-1.13.3**: Multiple checks per iteration are aggregated independently.
- **AC-1.13.4**: Console and JSON reports display per-check pass/fail counts and percentages.
- **AC-1.13.5**: SLA Thresholds evaluate `vuhive.checks.failed` and `vuhive.checks.passed`.

---

### 14.8 Increment 1.14 — Data Parameterization Module (`pkg/vuhive/data`)

Provide a dedicated data loading and parameterization package for CSV, JSON, and JSON Lines datasets with thread-safe distribution strategies.

#### Public API (`pkg/vuhive/data`)
- `type Record = map[string]string`
- `type Strategy int` (`Sequential`, `Random`, `UniquePerVU`, `SharedQueue`)
- `var ErrDatasetExhausted = errors.New(...)`
- `var ErrNilContext = errors.New(...)`
- `LoadCSV(r io.Reader, strategy Strategy) (*DataSet, error)` / `LoadCSVFile(path, strategy)`
- `LoadJSON(r io.Reader, strategy Strategy) (*DataSet, error)` / `LoadJSONFile(path, strategy)`
- `LoadJSONL(r io.Reader, strategy Strategy) (*DataSet, error)` / `LoadJSONLFile(path, strategy)`
- `(ds *DataSet) Next(ctx ContextAccessor) (Record, error)`

#### Acceptance Criteria
- **AC-1.14.1**: `LoadCSV` parses CSV with headers into string key-value maps.
- **AC-1.14.2**: `LoadJSON` parses JSON array of objects into key-value maps.
- **AC-1.14.3**: `LoadJSONL` parses newline-delimited JSON objects into key-value maps.
- **AC-1.14.4**: Sequential and UniquePerVU strategies round-robin / partition across rows deterministically by VU ID and iteration, returning `ErrNilContext` when `ctx == nil`.
- **AC-1.14.5**: Random strategy selects rows uniformly with lock-free thread safety (`math/rand/v2`).
- **AC-1.14.6**: SharedQueue strategy dispenses each row exactly once across concurrent VUs and returns `ErrDatasetExhausted` when depleted.

---

### 14.9 Increment 1.16 — Graceful Abort / Early Stop (`abort_on_fail`)

Support early test termination when critical thresholds breach during test execution, avoiding wasted resources or runaway system damage.

#### Configuration Options
- `ThresholdConfig.AbortOnFail` (`abort_on_fail: true`): Enables real-time periodic evaluation during test execution.
- `ThresholdConfig.DelayAbortEval` (`delay_abort_eval: 5s`): Optional warm-up grace period before abort evaluation begins.

#### Acceptance Criteria
- **AC-1.16.1**: Threshold configured with `abort_on_fail=true` evaluates periodically during test execution.
- **AC-1.16.2**: Breach before `delay_abort_eval` is ignored (warm-up grace period).
- **AC-1.16.3**: Breach after `delay_abort_eval` cancels all active VU contexts immediately.
- **AC-1.16.4**: Aborted test generates report showing `ABORTED` status and exits with code 1.




