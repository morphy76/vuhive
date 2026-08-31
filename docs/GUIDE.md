# vuhive Developer Guide

A step-by-step guide for load test developers adopting the `vuhive` framework.

---

## Table of Contents

1. [Getting Started](#1-getting-started)
2. [Project Structure](#2-project-structure)
3. [Writing Your First Scenario](#3-writing-your-first-scenario)
4. [Configuration (vuhive.yaml)](#4-configuration-vuhiveyaml)
5. [Context Hierarchy & Role-Specific Interfaces](#5-context-hierarchy--role-specific-interfaces)
6. [Lifecycle Hooks Deep Dive](#6-lifecycle-hooks-deep-dive)
7. [Recording Metrics](#7-recording-metrics)
8. [SLA Thresholds (Quality Gates)](#8-sla-thresholds-quality-gates)
9. [Pacing Modes](#9-pacing-modes)
10. [CLI Flags & Execution](#10-cli-flags--execution)
11. [Patterns & Recipes](#11-patterns--recipes)
12. [Troubleshooting](#12-troubleshooting)
13. [Reference Implementations (examples/)](#13-reference-implementations-examples)
14. [Performance & Framework Overhead Optimization](#14-performance--framework-overhead-optimization)

---

## 1. Getting Started

### Prerequisites

- Go 1.26+
- A target system to load test (HTTP, gRPC, SSE, WebSocket, etc.)

### Install

```bash
mkdir my-load-test && cd my-load-test
go mod init myorg/my-load-test
go get github.com/morphy76/vuhive
```

### Minimal example

Create `main.go`:

```go
//go:build vuhive_example

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/morphy76/vuhive/pkg/vuhive"
)

func main() {
	suite := vuhive.NewSuite("My First Load Test")

	suite.RegisterScenario("hello", vuhive.Scenario{
		RunVU: func(ctx vuhive.VUContext) error {
			start := time.Now()
			time.Sleep(10 * time.Millisecond) // replace with real work
			ctx.Metrics().Duration("response_time", vuhive.Tags{}).Observe(time.Since(start))
			ctx.Metrics().Counter("requests", vuhive.Tags{}).Inc()
			return nil
		},
	})

	res := suite.Execute()
	if res.Error != nil {
		fmt.Fprintf(os.Stderr, "Execution error: %v\n", res.Error)
	}
	os.Exit(res.ExitCode())
}
```

Create `vuhive.yaml`:

```yaml
version: "1.0"
default_scenario: hello

scenarios:
  hello:
    type: constant_vus
    vus: 5
    run_period: 10s
    vu_timeout: 2s
```

Run:

```bash
go run main.go
```

---

## 2. Project Structure

vuhive is a **library** — you write a Go `main` package that imports it, compile it, and run the binary. There is no separate CLI tool.

### Recommended layout

```text
my-load-test/
├── go.mod
├── main.go              ← registers scenarios, calls suite.Execute()
├── vuhive.yaml           ← load profile configuration
├── dsl/                 ← optional: domain-specific helpers (HTTP client, SSE, CSV loader)
│   ├── client.go
│   ├── metrics.go
│   └── flow.go
└── data/
    └── prompts.csv      ← optional: test data files
```

### Key principle

| Concern | Where |
|---------|-------|
| **What** to test (business logic) | `main.go` / `dsl/` (Go code) |
| **How much** load (concurrency, duration) | `vuhive.yaml` (YAML config) |
| **Pass/fail criteria** (SLAs) | `vuhive.yaml` → `thresholds` block |

---

## 3. Writing Your First Scenario

A scenario is a struct with up to 6 lifecycle hooks. Only `RunVU` is required:

```go
suite.RegisterScenario("checkout_flow", vuhive.Scenario{
    // (1) Setup — runs ONCE before any VU spawns
    Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
        client := &http.Client{Timeout: 5 * time.Second}
        return map[string]any{"client": client}, nil
    },

    // (2) PreTest — runs ONCE per VU before its iteration loop
    PreTest: func(ctx vuhive.VUContext) error {
        ctx.Log().Debug().Int64("vu", ctx.VUID()).Msg("VU starting")
        return nil
    },

    // (3) RunVU — called repeatedly in a loop per VU during run_period
    RunVU: func(ctx vuhive.VUContext) error {
        baseURL := ctx.Param("base_url")
        client := ctx.GlobalState("client").(*http.Client)

        start := time.Now()
        req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/health", nil)
        resp, err := client.Do(req)
        elapsed := time.Since(start)

        ctx.Metrics().Duration("http_latency", vuhive.Tags{"endpoint": "/health"}).Observe(elapsed)

        if err != nil {
            ctx.Metrics().Counter("errors", vuhive.Tags{}).Inc()
            return err
        }
        defer func() {
            _ = resp.Body.Close()
        }()


        ctx.Metrics().Counter("requests", vuhive.Tags{"status": fmt.Sprint(resp.StatusCode)}).Inc()
        return nil
    },

    // (4) AfterTest — runs ONCE per VU after loop ends (defer-guaranteed)
    AfterTest: func(ctx vuhive.VUContext) error {
        ctx.Log().Info().Int64("vu", ctx.VUID()).Msg("VU finished")
        return nil
    },

    // (5) Teardown — runs ONCE after ALL VUs exit
    Teardown: func(ctx vuhive.TeardownContext, state map[string]any) error {
        ctx.Log().Info().Msg("cleaning up shared resources")
        return nil
    },

    // (6) HandleSummary — runs ONCE after report generation with structured results
    HandleSummary: func(ctx vuhive.SummaryContext, summary vuhive.SummaryData) error {
        fmt.Printf("Scenario %s ended in %v, passed=%v\n", summary.Scenario, summary.Duration, summary.Passed)
        return nil
    },
})
```


---

## 4. Configuration (vuhive.yaml)

All load profile settings are declarative YAML. Your Go code reads them via `ctx.Param()`.

### Complete field reference

```yaml
version: "1.0"                  # required, must be "1.0"
default_scenario: my_scenario   # optional, fallback when --scenario not passed

scenarios:
  my_scenario:
    type: constant_vus           # "constant_vus", "arrival_rate", or "ramping_vus"

    # --- constant_vus fields ---
    vus: 10                      # number of concurrent VUs (required for constant_vus)
    ramp_up: 5s                  # linear stagger VU spawn (optional, default 0)
    run_period: 30s              # steady-state duration (required)
    ramp_down: 5s                # active iteration dispatch ramp-down (optional, default 0)
    drain: 10s                   # grace period for in-flight VUs to complete before termination (optional, default 0)
    vu_timeout: 2s               # per-iteration context deadline (required)


    # --- arrival_rate fields ---
    # target_tps: 100            # target transactions/sec (required for arrival_rate)
    # max_vus: 50                # worker pool cap (required for arrival_rate)

    # --- http (declarative HTTP client configuration) ---
    http:
      base_url: "https://api.example.com" # prepended to relative paths in client.Get/Post/etc.
      timeout: 5s                         # per-request timeout (default 30s)
      headers:                            # default headers sent with every request
        Accept: "application/json"
        User-Agent: "vuhive/1.0"
      tls:
        insecure_skip_verify: false       # disable certificate verification (default false)
      pool:
        max_idle_conns: 100               # max idle connections across all hosts
        max_idle_conns_per_host: 10       # max idle connections per host
        idle_conn_timeout: 90s            # idle connection keep-alive timeout
      detailed_timing: false              # enable httptrace per-phase metrics (default false)
      metric_prefix: "vuhive.http."       # custom prefix (default "vuhive.http.")

    # --- params (available via ctx.Param) ---
    params:
      checkout_path: "/api/checkout"
      timeout_ms: "500"
      messages_file: "data/prompts.csv"

    # --- interaction_delay / think_time (thinking time strategy invoked via ctx.Sleep()) ---
    interaction_delay:
      type: range                # "fixed", "range", "expo", or "gaussian"
      min: 200ms
      max: 1s


    # --- SLA thresholds ---
    thresholds:

      - metric: http_latency
        stat: p95
        operator: "<"
        target: "200ms"
      - metric: requests
        stat: count
        operator: ">="
        target: "1000"
```

### Duration format

All duration fields use Go's `time.ParseDuration` format: `50ms`, `1s`, `5m`, `1h30m`.

### IDE Autocompletion & Schema Validation

`vuhive` provides an official JSON Schema (`schemas/vuhive.schema.json`) that enables IntelliSense, field auto-completion, real-time validation, and rich documentation tooltips across VS Code, GoLand / IntelliJ IDEA, Cursor, Neovim, and other editors.

#### Option 1: In-File Model Directive (Zero IDE Setup)

Add the standard schema comment at the top of your `vuhive.yaml` file:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/morphy76/vuhive/main/schemas/vuhive.schema.json
version: "1.0"
default_scenario: http_checkout_flow

scenarios:
  http_checkout_flow:
    type: constant_vus
    vus: 10
    run_period: 30s
    vu_timeout: 2s
```

#### Option 2: VS Code Workspace Settings (`.vscode/settings.json`)

Configure schema mapping for all `vuhive.yaml` files automatically via YAML language server:

```json
{
  "yaml.schemas": {
    "https://raw.githubusercontent.com/morphy76/vuhive/main/schemas/vuhive.schema.json": [
      "vuhive.yaml",
      "vuhive.yml",
      "*vuhive*.yaml",
      "*vuhive*.yml"
    ]
  }
}
```

#### Option 3: JetBrains (GoLand / IntelliJ IDEA)

1. Open **Settings** / **Preferences** (`Cmd+,` on macOS or `Ctrl+Alt+S` on Linux/Windows).
2. Navigate to **Languages & Frameworks** > **Schemas and DTDs** > **JSON Schema Mappings**.
3. Click `+` to add a new mapping:
   - **Name**: `vuhive Configuration Schema`
   - **Schema file or URL**: `https://raw.githubusercontent.com/morphy76/vuhive/main/schemas/vuhive.schema.json`
   - **Schema version**: `JSON Schema version 7` or `2020-12`
   - **File path pattern**: `*vuhive*.yaml;*vuhive*.yml`

---

## 5. Context Hierarchy & Role-Specific Interfaces

To adhere strictly to the **Interface Segregation Principle (ISP)** and prevent meaningless/dummy method calls during non-VU lifecycle phases, the framework aggregates granular capability interfaces into role-specific composed context interfaces:

### Role-Specific Composed Interfaces

| Context Interface | Lifecycle Phase | Composed Capability Interfaces | Available Capabilities |
|-------------------|-----------------|--------------------------------|------------------------|
| **`SetupContext`** | `Setup` | `context.Context`, `ConfigProvider`, `ObservabilityProvider` | Params, Structured Logging, Metrics |
| **`VUContext`** | `PreTest`, `RunVU`, `AfterTest` | `context.Context`, `ExecutionIdentity`, `ConfigProvider`, `StateProvider`, `ObservabilityProvider`, `WorkflowController` | VUID, Iteration, Params, GlobalState, Log, Metrics, Sleep, Check |
| **`TeardownContext`** | `Teardown` | `context.Context`, `ConfigProvider`, `StateProvider`, `ObservabilityProvider` | Params, GlobalState (read-only), Log, Metrics |
| **`SummaryContext`** | `HandleSummary` | `context.Context`, `ConfigProvider`, `ObservabilityProvider` | Params, Structured Logging, Cancellation |
| **`ScenarioContext`** | *Backward compatibility* | *Identical to `VUContext`* | Full VU capability set |

### Capability Interfaces Reference

| Method | Capability Interface | Return | Description |
|--------|----------------------|--------|-------------|
| `VUID()` | `ExecutionIdentity` | `int64` | 1-based virtual user ID |
| `Iteration()` | `ExecutionIdentity` | `int64` | 0-based iteration count (0 in PreTest/AfterTest) |
| `ScenarioName()` | `ExecutionIdentity` | `string` | Active scenario name from YAML |
| `Param(key)` | `ConfigProvider` | `string` | Read scenario `params` by key; `""` if absent |
| `ParamInt(key, default)` | `ConfigProvider` | `int` | Parse param as int; logs warning and returns default on parse failure |
| `ParamDuration(key, default)` | `ConfigProvider` | `time.Duration` | Parse param as duration; logs warning and returns default on parse failure |
| `HTTPConfig()` | `ConfigProvider` | `HTTPConfig` | Typed declarative HTTP client configuration (BaseURL, Timeout, Headers, TLS, Pool) |
| `GlobalState(key)` | `StateProvider` | `any` | Read value from Setup's returned map (shallow-copied, read-only) |
| `Log()` | `ObservabilityProvider` | `Logger` | Zerolog logger pre-enriched with scenario/VU/iteration context |
| `Metrics()` | `ObservabilityProvider` | `MetricsCollector` | Record custom counters, gauges, durations, rates |
| `Sleep(d ...time.Duration)` | `WorkflowController` | `error` | Pause for explicit duration or scenario `interaction_delay` strategy (respects `ctx.Done()`) |
| `Check(name, fn)` | `WorkflowController` | `bool` | Evaluate inline pass/fail assertion (`CheckFunc`) without stopping VU iteration |
| `Group(name, fn)` | `WorkflowController` | `error` | Execute `fn` within a named transaction boundary with automatic latency measurement |


### Interface Segregation & Modular Helpers

Because context interfaces are decomposed into discrete interfaces, helper functions and custom components can accept only the specific capability they need instead of depending on the entire fat context:

```go
// Accepts only execution identity (e.g. for deterministic data partition or logging correlation)
func ProcessUserBatch(id vuhive.ExecutionIdentity, data []string) string {
    idx := (id.VUID() - 1 + id.Iteration()) % int64(len(data))
    return data[idx]
}

// Accepts only configuration parameters
func BuildClientURL(cfg vuhive.ConfigProvider) string {
    return fmt.Sprintf("%s/api/v1", cfg.Param("base_url"))
}

// Accepts only workflow controls (sleep / check)
func PerformHealthCheck(wf vuhive.WorkflowController, client *http.Client, url string) bool {
    return wf.Check("endpoint reachable", func() string {
        resp, err := client.Get(url)
        if err != nil || resp.StatusCode != http.StatusOK {
            return "health check failed"
        }
        return ""
    })
}
```

This also makes unit testing helper functions trivial, as test authors only need to stub 1-3 methods rather than the full context.

### Using as context.Context

Because all role-specific context interfaces embed standard Go `context.Context`, you can pass them directly to stdlib and third-party calls:

```go
req, _ := http.NewRequestWithContext(ctx, "POST", url, body)
resp, err := client.Do(req)
```

The context carries the `vu_timeout` deadline during VU iterations. When it expires, `ctx.Err() == context.DeadlineExceeded`.

---

## 6. Lifecycle Hooks Deep Dive

### Execution order

```text
Setup(ctx SetupContext) ─────────────────── runs once
  │
  ├── VU 1: PreTest(VUContext) → RunVU(VUContext) loop → AfterTest(VUContext) [defer]
  ├── VU 2: PreTest(VUContext) → RunVU(VUContext) loop → AfterTest(VUContext) [defer]
  └── VU N: PreTest(VUContext) → RunVU(VUContext) loop → AfterTest(VUContext) [defer]
  │
Teardown(ctx TeardownContext, state) ───── runs once
  │
HandleSummary(ctx SummaryContext, summary) ─ runs once (post-report generation)
```

### Key guarantees

| Hook | Parameter Signature | Cardinality | Error behavior |
|------|---------------------|-------------|----------------|
| **Setup** | `ctx SetupContext` | Once | Non-nil error **aborts** entire test (returns `*SetupError`) |
| **PreTest** | `ctx VUContext` | Once per VU | Non-nil error **skips** RunVU; AfterTest still runs |
| **RunVU** | `ctx VUContext` | Loop per VU | Errors and panics are **caught, logged, counted**; loop continues |
| **AfterTest** | `ctx VUContext` | Once per VU | Runs via `defer` — **guaranteed** even after RunVU panic |
| **Teardown** | `ctx TeardownContext, state map[string]any` | Once | Error is **logged** but does not affect pass/fail verdict |
| **HandleSummary** | `ctx SummaryContext, summary SummaryData` | Once | Error is **logged** but does not affect exit code or SLA verdict |


### Panic recovery

If `RunVU` panics, the framework:
1. Catches the panic via `recover()`
2. Increments `vuhive.vu.panics` and `vuhive.vu.iterations_failed`
3. Logs the panic value
4. **Continues the iteration loop** (the VU is not killed)

### GlobalState contract & Thread Safety

- Setup returns `map[string]any` → the framework makes a **shallow copy** of this map before starting VUs.
- All VUs share the **same copied map** (read-only) via `ctx.GlobalState(key)`.
- **Shallow Copy Limitation**: The shallow copy protects the top-level map keys from mutation, but does **not** perform a deep copy of nested mutable objects (e.g., slices, inner maps, pointer structures).
- **Thread Safety Invariant**: If `Setup` returns complex nested structures or pointers, they must be treated as **immutable** by all VUs, or access must be protected using standard Go concurrency primitives (`sync.Mutex`, `sync.RWMutex`, atomic values, or thread-safe types like `sync.Map`).
- **Do not mutate** shared state from VU code without synchronization.

---

## 7. Recording Metrics

### Four metric types

```go
// Duration — HDR histogram for latency percentiles
ctx.Metrics().Duration("api_latency", vuhive.Tags{"route": "/checkout"}).Observe(elapsed)

// Counter — monotonically increasing integer
ctx.Metrics().Counter("api_requests", vuhive.Tags{"status": "200"}).Inc()
ctx.Metrics().Counter("bytes_sent", vuhive.Tags{}).Add(4096)

// Gauge — instantaneous snapshot value
ctx.Metrics().Gauge("active_connections", vuhive.Tags{}).Set(42)
ctx.Metrics().Gauge("active_connections", vuhive.Tags{}).Add(-1)

// Rate — explicit numerator/denominator ratio
ctx.Metrics().Rate("success_rate", vuhive.Tags{}).Add(1, 1) // 1 success out of 1 attempt
ctx.Metrics().Rate("success_rate", vuhive.Tags{}).Add(0, 1) // 0 successes out of 1 attempt
```

### Tags

Tags let you slice metrics by dimension. Same metric name + different tags = separate time series, merged at report time for SLA evaluation.

```go
ctx.Metrics().Duration("api_latency", vuhive.Tags{"endpoint": "/login"}).Observe(d1)
ctx.Metrics().Duration("api_latency", vuhive.Tags{"endpoint": "/checkout"}).Observe(d2)
```

The SLA threshold `api_latency.p95 < 200ms` evaluates the **merged** histogram across all tag variants.

### Type collision protection

A metric name is locked to its first-registered type. Attempting to use the same name as a different type panics:

```go
ctx.Metrics().Counter("foo", vuhive.Tags{}).Inc()
ctx.Metrics().Duration("foo", vuhive.Tags{}).Observe(d) // PANIC: "foo" already registered as Counter
```

### Built-in metrics (auto-recorded by the framework)

All built-in metrics are exported as typed constants in `pkg/vuhive` (e.g. `vuhive.MetricIterationsTotal`) and `internal/metric`:

| Constant (`pkg/vuhive`) | String Identifier | Type | Description |
|------------------------|-------------------|------|-------------|
| `vuhive.MetricIterationsTotal` | `vuhive.vu.iterations_total` | Counter | Total iterations executed |
| `vuhive.MetricIterationsFailed` | `vuhive.vu.iterations_failed` | Counter | Iterations that errored or panicked |
| `vuhive.MetricIterationsTimeout` | `vuhive.vu.iterations_timeout` | Counter | Iterations exceeding `vu_timeout` |
| `vuhive.MetricVUPanics` | `vuhive.vu.panics` | Counter | RunVU panics recovered |
| `vuhive.MetricVUPretestErrors` | `vuhive.vu.pretest_errors` | Counter | PreTest hook failures |
| `vuhive.MetricVUActive` | `vuhive.vu.active` | Gauge | Currently active VU goroutines |
| `vuhive.MetricIterationDuration` | `vuhive.vu.iteration_duration` | Duration | Completed VU iteration duration |
| `vuhive.MetricPacingDroppedIterations` | `vuhive.pacing.dropped_iterations` | Counter | Arrival-rate tokens dropped due to pool saturation |
| `vuhive.MetricChecksPassed` | `vuhive.checks.passed` | Counter | Total inline checks that passed |
| `vuhive.MetricChecksFailed` | `vuhive.checks.failed` | Counter | Total inline checks that failed |
| `vuhive.MetricHTTPReqDuration` | `vuhive.http.req_duration` | Duration | Total HTTP request latency |
| `vuhive.MetricHTTPReqs` | `vuhive.http.reqs` | Counter | Total HTTP requests count |
| `vuhive.MetricHTTPReqFailed` | `vuhive.http.req_failed` | Rate | Failed HTTP request ratio |
| `vuhive.MetricHTTPReqConnecting` | `vuhive.http.req_connecting` | Duration | TCP connection establishment time *(opt-in)* |
| `vuhive.MetricHTTPReqTLSHandshaking` | `vuhive.http.req_tls_handshaking` | Duration | TLS handshake time *(opt-in)* |
| `vuhive.MetricHTTPReqSending` | `vuhive.http.req_sending` | Duration | HTTP request write time *(opt-in)* |
| `vuhive.MetricHTTPReqReceiving` | `vuhive.http.req_receiving` | Duration | HTTP response read time *(opt-in)* |
| `vuhive.MetricKafkaPubDuration` | `vuhive.kafka.pub_duration` | Duration | Kafka publish round-trip latency |
| `vuhive.MetricKafkaPubTotal` | `vuhive.kafka.pub_total` | Counter | Total Kafka messages published |
| `vuhive.MetricKafkaPubBytes` | `vuhive.kafka.pub_bytes` | Counter | Total Kafka payload bytes published |
| `vuhive.MetricKafkaPubFailed` | `vuhive.kafka.pub_failed` | Rate | Failed Kafka publish attempts ratio |
| `vuhive.MetricKafkaSubDuration` | `vuhive.kafka.sub_duration` | Duration | Kafka message fetch/wait duration |
| `vuhive.MetricKafkaSubTotal` | `vuhive.kafka.sub_total` | Counter | Total Kafka messages consumed |
| `vuhive.MetricKafkaSubBytes` | `vuhive.kafka.sub_bytes` | Counter | Total Kafka payload bytes consumed |
| `vuhive.MetricKafkaSubFailed` | `vuhive.kafka.sub_failed` | Rate | Failed Kafka consume attempts ratio |
| `vuhive.MetricGroupPrefix` | `vuhive.group.<path>.duration` | Duration | Per-step transaction group latency (nested with `::`) |

> **In-Flight Iterations on Shutdown:** `vuhive.vu.iterations_timeout` tracks only genuine per-iteration timeouts where `RunVU` exceeded `vu_timeout` during active execution. In-flight iterations that are interrupted when the overall scenario completes (`run_period` / `ramp_down` expiration or early abort) are cleanly cancelled and discarded without being counted as timeouts or failures.

### Transaction Boundaries (Groups)

When testing multi-step workflows (e.g. login, catalog search, checkout, payment), measuring overall iteration duration alone does not pinpoint which individual step degraded under load.

`ctx.Group(name, fn)` wraps an operation inside a named transaction boundary with automatic latency recording:

```go
suite.RegisterScenario("user_journey", vuhive.Scenario{
    RunVU: func(ctx vuhive.VUContext) error {
        // Step 1: Login
        err := ctx.Group("01_Login", func(ctx vuhive.VUContext) error {
            return performLogin(ctx)
        })
        if err != nil {
            return err
        }

        _ = ctx.Sleep() // Think time between steps

        // Step 2: Browse
        err = ctx.Group("02_Browse", func(ctx vuhive.VUContext) error {
            return browseCatalog(ctx)
        })
        if err != nil {
            return err
        }

        // Step 3: Checkout with nested sub-steps
        return ctx.Group("03_Checkout", func(ctx vuhive.VUContext) error {
            if err := ctx.Group("Cart", func(ctx vuhive.VUContext) error {
                return addToCart(ctx)
            }); err != nil {
                return err
            }

            return ctx.Group("Payment", func(ctx vuhive.VUContext) error {
                return submitPayment(ctx)
            })
        })
    },
})
```

#### Key Characteristics

1. **Metric Naming**: Generates HDR duration histograms named `vuhive.group.<path>.duration`. Nested groups concatenate paths using `::` (e.g. `vuhive.group.03_Checkout::Payment.duration`).
2. **Error & Panic Resilience**: If `fn` returns an error or panics, the duration up to that point is observed and recorded before propagating the error or recovering the panic.
3. **Dedicated Reporting**: Console summary outputs a `GROUPS` table with counts, min, mean, p95, p99, and max latencies. JSON reports include a structured `groups` array.
4. **SLA Thresholds**: Set latency SLA quality gates per transaction step directly in `vuhive.yaml`:
   ```yaml
   thresholds:
     - metric: "vuhive.group.01_Login.duration"
       stat: p95
       operator: "<"
       target: "200ms"
     - metric: "vuhive.group.03_Checkout::Payment.duration"
       stat: p95
       operator: "<"
       target: "300ms"
   ```

---

## 8. SLA Thresholds (Quality Gates)


Thresholds are declarative pass/fail assertions evaluated after the test run.

### Available stats

| Stat | Applicable to | Description |
|------|---------------|-------------|
| `p50`, `p90`, `p95`, `p99` | Duration | Percentile latency (target is `time.Duration` string) |
| `mean`, `max` | Duration | Mean/max latency |
| `count` | Counter | Total count (target is `float64`) |
| `rate` | Rate | `sum(numerator)/sum(denominator)` (target is `float64`) |
| `value` | Gauge | Last observed value (target is `float64`) |

### Operators

`<`, `<=`, `>`, `>=`

### Example

```yaml
thresholds:
  - metric: api_latency
    stat: p95
    operator: "<"
    target: "200ms"

  - metric: success_rate
    stat: rate
    operator: ">="
    target: "0.99"

  - metric: api_requests
    stat: count
    operator: ">="
    target: "1000"

  - metric: authentication_errors
    stat: count
    operator: "<="
    target: "0"
    on_no_data: zero  # Default for count/value; passes when 0 errors occur
```

### 8.1 Missing Data Handling (`on_no_data`)

When a load test executes cleanly and zero events are recorded for custom error counters (e.g. `ctx.Counter("auth_errors").Inc()` is never invoked), vuhive evaluates missing metric data according to configurable `on_no_data` policies:

| Strategy | Applicable Defaults | Behavior on Missing / Unrecorded Metric Data |
|---|---|---|
| `zero` | Default for `count` & `value` | Evaluates missing metrics with numerical value `0` (or `0s` for durations). Zero-tolerance checks such as `count <= 0` evaluate to `0 <= 0` and pass. |
| `fail` | Default for duration & `rate` | Treats absence of data as an automatic SLA failure (`actual: no data`). |
| `pass` | Optional | Marks the threshold as passed if no events/metrics were recorded (`actual: no data`). |
| `ignore` / `skip` | Optional | Excludes/ignores the threshold from causing SLA failure when no data was emitted (`actual: no data`). |

```yaml
thresholds:
  # Option A: Zero-tolerance error counter (passes when no errors occur)
  - metric: database_deadlocks
    stat: count
    operator: "<="
    target: "0"
    on_no_data: zero

  # Option B: Duration metric requiring completed requests
  - metric: http_request_duration
    stat: p95
    operator: "<"
    target: "200ms"
    on_no_data: fail

  # Option C: Optional branch metric
  - metric: cache_rebuild_duration
    stat: p95
    operator: "<"
    target: "1s"
    on_no_data: pass
```

### 8.2 Early Stop / Graceful Abort (`abort_on_fail`)

Configure `abort_on_fail: true` on any threshold entry to immediately terminate test execution if that threshold is breached during execution, preventing wasted test execution or runaway system degradation.

Optionally specify `delay_abort_eval` to establish a warm-up grace period before early termination monitoring begins:

```yaml
thresholds:
  - metric: vuhive.vu.iterations_failed
    stat: count
    operator: "<="
    target: "0"
    abort_on_fail: true
    delay_abort_eval: 5s   # Ignore failures during first 5 seconds of ramp-up

  - metric: http_request_duration
    stat: p95
    operator: "<"
    target: "500ms"
    abort_on_fail: true
```

When an early stop is triggered:
1. All active VU contexts are cancelled immediately (`ctx.Done()`).
2. The terminal summary report displays `OVERALL: ABORTED (exit 1)` along with the triggering threshold reason.
3. The process exits with code `1`.

### Exit codes

- **Exit 0**: All thresholds pass → `OVERALL: PASSED`
- **Exit 1**: Any threshold fails → `OVERALL: FAILED`

This makes vuhive suitable for CI/CD pipelines where exit code drives the build status.

---

## 9. Pacing Modes

### constant_vus (Closed System)

Maintains a **fixed pool** of VU goroutines. Each VU runs RunVU in a loop until `run_period` expires.

```text
Time ──────────────────────────────────────────────────────────►
     │← ramp_up →│← run_period ────────→│← ramp_down →│← drain →│

VU 1 ·············▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒▒▒▒▒▒▒▒▒▒▒▒▒▒░░░░░░░
VU 2 ··············▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒▒▒▒▒▒▒▒▒▒▒▒▒▒░░░░░░░
VU 3 ···············▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒▒▒▒▒▒▒▒▒▒▒▒▒▒░░░░░░░

· = waiting during ramp_up stagger
▓ = actively running iterations
▒ = ramp_down dispatch phase
░ = drain phase (grace period for in-flight iterations to complete)
```

**Best for**: simulating a fixed number of concurrent users.

> **Graceful Completion & Drain:** When active execution ends (`ramp_up + run_period + ramp_down`), VUs stop starting new iterations. The engine enters the `drain` phase: in-flight iterations are allowed up to `drain` duration to finish cleanly. If all VUs finish early, the engine returns immediately. Any iterations still in-flight after `drain` expires (or immediately if `drain: 0s`) are cleanly cancelled and discarded without false timeout or error reporting.

### arrival_rate (Open System)

Dispatches iterations at a target TPS using a token bucket. A bounded worker pool (`max_vus`) prevents goroutine explosion.

```yaml
type: arrival_rate
target_tps: 100       # 100 iterations/second
max_vus: 50           # max concurrent workers
ramp_up: 10s
run_period: 1m
ramp_down: 5s         # optional dispatch ramp-down
drain: 10s            # optional grace period for in-flight workers
vu_timeout: 2s
```

If all `max_vus` workers are busy when a token arrives, the iteration is **dropped** and `vuhive.pacing.dropped_iterations` is incremented. In-flight workers completing during the `drain` phase are recorded normally; remaining active workers after drain timeout are discarded without false timeout or error reporting.

**Best for**: testing at a specific throughput regardless of response time.

### ramping_vus (Multi-Stage Pacing)

Dynamically ramps and adjusts VU count across multiple stages over time. Ideal for spike testing, step functions, and load curves.

```yaml
type: ramping_vus
stages:
  - target: 10
    duration: 30s      # ramp up from 0 to 10 VUs over 30s
  - target: 10
    duration: 1m       # hold steady at 10 VUs for 1 minute
  - target: 50
    duration: 10s      # spike to 50 VUs over 10s
  - target: 50
    duration: 2m       # hold spike for 2 minutes
  - target: 0
    duration: 30s      # ramp down to 0 VUs
ramp_down: 5s          # optional stage ramp-down
drain: 10s             # optional grace period for remaining workers
vu_timeout: 2s         # per-iteration timeout (required)
```

The engine continuously tracks stage progress and dynamically scales the active virtual user pool up or down to match the calculated target. In-flight iterations completing gracefully during the `drain` phase are recorded normally; remaining active workers exceeding `drain` timeout are cleanly terminated upon test completion.

**Best for**: simulating realistic traffic spikes, stress testing breaking points, and observing system recovery.


---

## 10. CLI Flags & Execution

```bash
go run main.go --config vuhive.yaml --scenario my_scenario
```

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `vuhive.yaml` | Path to config file |
| `--scenario` | *(from config)* | Scenario name to run |
| `--log-level` | `info` | `debug`, `info`, `warn`, `error` |
| `--log-format` | `pretty` | `pretty` (human-readable) or `json` |
| `--report-format` | `console` | `console` or `json` |
| `--report-out` | *(stdout)* | Write report to file |
| `--json-report-out` | *(disabled)* | Write additional JSON report to file |
| `--version` | `false` | Print version and exit |

---

## 11. Patterns & Recipes

### 11.1 HTTP Load Test with Tags

```go
RunVU: func(ctx vuhive.VUContext) error {
    endpoints := []string{"/api/users", "/api/orders", "/api/products"}
    for _, ep := range endpoints {
        start := time.Now()
        req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+ep, nil)
        resp, err := client.Do(req)
        elapsed := time.Since(start)

        tags := vuhive.Tags{"endpoint": ep}
        ctx.Metrics().Duration("http_latency", tags).Observe(elapsed)

        if err != nil || resp.StatusCode >= 400 {
            ctx.Metrics().Counter("http_errors", tags).Inc()
            ctx.Metrics().Rate("success_rate", vuhive.Tags{}).Add(0, 1)
        } else {
            _ = resp.Body.Close()
            ctx.Metrics().Rate("success_rate", vuhive.Tags{}).Add(1, 1)
        }

    }
    return nil
},
```

### 11.2 Built-in Instrumented HTTP Client (`pkg/vuhive/http`)

For standard HTTP load testing, use the built-in `github.com/morphy76/vuhive/pkg/vuhive/http` package to eliminate manual metric timing and counting boilerplate.

#### Why Use `pkg/vuhive/http`?
- **Automatic Metrics**: Automatically records `vuhive.http.req_duration` (HDR latency histogram), `vuhive.http.reqs` (total counter), and `vuhive.http.req_failed` (failure rate) tagged with `method`, `url` (path only), and `status`.
- **Declarative YAML Configuration**: Configure base URL, timeouts, default headers, connection pool settings, and TLS options directly in `vuhive.yaml` under `scenarios.<name>.http`.
- **Zero Boilerplate (`vuhivehttp.Default(ctx)`)**: Retrieve the scenario's shared HTTP client directly in `RunVU` without needing any `Setup` hook or type-casting from `GlobalState`.
- **Convenient Constructors**: Also supports explicit initialization from context using `vuhivehttp.NewClientFromConfig(ctx)` (in `Setup`) or `vuhivehttp.NewClientFromVUConfig(ctx)` (in `RunVU`).
- **Relative Path Resolution**: When `base_url` is configured, `client.Get(ctx, "/path")` automatically resolves against the base URL. Absolute URLs (`https://...`) are preserved as-is.
- **Response Decoding**: `resp.JSON(&target)` and `resp.Text()` methods with automatic response body cleanup.
- **Opt-in Detailed Timing**: Capture TCP connection, TLS handshake, request write, and response read latencies via `detailed_timing: true` in YAML or `vuhivehttp.WithDetailedTiming()`.

#### Declarative Configuration (`vuhive.yaml`)

```yaml
scenarios:
  catalog_api:
    type: constant_vus
    vus: 20
    run_period: 30s
    vu_timeout: 5s

    http:
      base_url: "https://api.example.com"
      timeout: 5s
      headers:
        Accept: "application/json"
        User-Agent: "vuhive/1.0"
      pool:
        max_idle_conns: 100
        max_idle_conns_per_host: 50
        idle_conn_timeout: 90s
      detailed_timing: false
```

#### Example Usage

```go
package main

import (
	"fmt"
	"net/http"

	"github.com/morphy76/vuhive/pkg/vuhive"
	vuhivehttp "github.com/morphy76/vuhive/pkg/vuhive/http"
)

type ItemResponse struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func main() {
	suite := vuhive.NewSuite("HTTP Load Test")

	suite.RegisterScenario("catalog_api", vuhive.Scenario{
		// Zero Setup boilerplate: Default(ctx) automatically uses vuhive.yaml config
		RunVU: func(ctx vuhive.VUContext) error {
			client := vuhivehttp.Default(ctx)

			// 1. Execute GET request — relative path resolved against base_url, metrics auto-recorded
			resp, err := client.Get(ctx, "/api/items/item-123")
			if err != nil {
				return err
			}

			// 2. Perform inline check on status code
			ctx.Check("status_200", func() string {
				if resp.StatusCode != http.StatusOK {
					return fmt.Sprintf("expected 200, got %d", resp.StatusCode)
				}
				return ""
			})

			// 3. Decode JSON body directly into struct
			var item ItemResponse
			if err := resp.JSON(&item); err != nil {
				return fmt.Errorf("failed to decode response: %w", err)
			}

			ctx.Check("valid_item_id", func() string {
				if item.ID != "item-123" {
					return fmt.Sprintf("unexpected item ID: %s", item.ID)
				}
				return ""
			})

			return nil
		},
	})

	suite.Execute()
}
```

#### Real-Time Server-Sent Events (SSE) Streaming

For continuous real-time streaming architectures (LLM token generation, live market tickers, real-time push notifications), use `client.StreamSSE` or `client.DoStream`. It automatically manages W3C EventSource framing (`text/event-stream`), disables total-lifecycle timeouts while preserving socket connect timeouts, and provides both iterator-based (`stream.Next()`) and channel-based (`stream.Events()`) consumption without unbounded memory buffering.

> **Context & Deadline Handling:** `DoStream` automatically **detaches short parent iteration deadlines** (`vu_timeout`) using `context.WithoutCancel`, so long-lived SSE streams are not prematurely killed when the per-iteration timeout expires. Explicit parent context cancellation (e.g. scenario teardown, VU shutdown) still cleanly terminates active streams. If `ctx` is `nil` or `context.Background()`, `DoStream` falls back to the request's own context (`req.Context()`). A pre-canceled context returns an immediate error without dialing.

```go
RunVU: func(ctx vuhive.VUContext) error {
    client := vuhivehttp.Default(ctx)

    // Open SSE stream — "Accept: text/event-stream" is set automatically
    // The stream automatically survives beyond vu_timeout deadlines.
    stream, err := client.StreamSSE(ctx, "/v1/chat/completions/stream")
    if err != nil {
        return fmt.Errorf("failed to open SSE stream: %w", err)
    }
    defer stream.Close()

    var tokenCount int
    for stream.Next() {
        event := stream.Event()
        if event.Event == "token" {
            tokenCount++
        }
        if event.Data == "[DONE]" {
            break
        }
    }

    if err := stream.Err(); err != nil {
        return fmt.Errorf("stream read error: %w", err)
    }

    ctx.Check("received_tokens", tokenCount > 0)
    return nil
}
```

#### Third-Party SDK Integration (`StandardClient` & `Transport`)

If your load test uses an external Go client library (such as an LLM provider SDK, custom gRPC-gateway client, or third-party HTTP client) that requires a standard `*http.Client` or `http.RoundTripper`, use `client.StandardClient()` or `client.Transport()`:

```go
RunVU: func(ctx vuhive.VUContext) error {
    // 1. Obtain standard *http.Client backed by vuhive instrumentation
    stdClient := vuhivehttp.Default(ctx).StandardClient()

    // 2. Pass into third-party SDK constructor
    sdkClient := thirdparty.NewClient(
        thirdparty.WithHTTPClient(stdClient),
        thirdparty.WithBaseURL("https://api.example.com"),
    )

    // 3. Execute requests or streaming calls via the SDK
    return sdkClient.StreamCompletions(ctx, prompt)
}
```

- **Transparent SSE Streaming**: Requests specifying `Accept: text/event-stream` are automatically executed via `DoStream` with detached deadlines, streaming chunks through the standard `*http.Response.Body`.
- **Zero-Boilerplate Telemetry**: Full `vuhive.http.*` and `vuhive.http.sse.*` metrics are collected without requiring custom adapters.
- **Safe Socket Cleanup**: When the SDK calls `resp.Body.Close()`, the underlying `SSEStream` is immediately terminated.

#### Wrapping Existing Standard `*http.Client` & `http.RoundTripper` (`vuhivehttp.Instrument`)

When bringing your own pre-configured `*http.Client` (e.g. AWS SDK, Google Cloud Client, Stripe, custom OAuth2 or mTLS clients), use `vuhivehttp.Instrument()` or `vuhivehttp.InstrumentTransport()` to wrap your client with automatic `vuhive` telemetry. Metrics are extracted dynamically per request from `req.Context()`:

```go
suite.RegisterScenario("custom_sdk_load_test", vuhive.Scenario{
    Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
        // 1. Create your pre-configured standard client (e.g. with custom timeouts, transport, or TLS)
        baseClient := &http.Client{Timeout: 5 * time.Second}

        // 2. Wrap it with vuhive telemetry instrumentation
        instrumented := vuhivehttp.Instrument(
            baseClient,
            vuhivehttp.WithMetricPrefix("vuhive.http."),
            vuhivehttp.WithTags(vuhive.Tags{"env": "staging"}),
        )

        // 3. Initialize third-party SDK using the instrumented client
        awsClient := customsdk.New(instrumented)
        return map[string]any{"sdk": awsClient}, nil
    },

    RunVU: func(ctx vuhive.VUContext) error {
        sdk := ctx.GlobalState("sdk").(*customsdk.Client)

        // Requests executed with ctx automatically record duration, count, and failure rates
        return sdk.CallAPI(ctx, "items/search")
    },
})
```

##### Supported Instrumentation Options:
- `vuhivehttp.WithMetricPrefix(prefix string)`: Sets the metric name prefix (defaults to `"vuhive.http."`).
- `vuhivehttp.WithInstrumentDetailedTiming(enabled ...bool)`: Enables collection of `httptrace` phase timings (`req_connecting`, `req_tls_handshaking`, `req_sending`).
- `vuhivehttp.WithTags(tags vuhive.Tags)`: Attaches static default tags to all metric observations from this client.

#### Auto-Recorded HTTP & SSE Metrics


| Metric Identifier | Type | Tags | Description |
|---|---|---|---|
| `vuhive.http.req_duration` | Duration (HDR) | `method`, `url`, `status` | Total request latency histogram |
| `vuhive.http.reqs` | Counter | `method`, `url`, `status` | Total HTTP requests count |
| `vuhive.http.req_failed` | Rate | `method`, `url`, `status` | Ratio of failed requests (non-2xx or transport error) |
| `vuhive.http.sse.connections_total` | Counter | `method`, `url`, `status` | Total SSE stream connection attempts |
| `vuhive.http.sse.connect_duration` | Duration (HDR) | `method`, `url`, `status` | Latency to establish connection and receive HTTP headers |
| `vuhive.http.sse.events_total` | Counter | `method`, `url`, `event_type` | Total number of received SSE events |
| `vuhive.http.sse.event_latency` | Duration (HDR) | `method`, `url`, `event_type` | Inter-arrival latency between successive events (TTFE) |
| `vuhive.http.sse.stream_duration` | Duration (HDR) | `method`, `url`, `status` | Total active lifespan of streaming sessions |
| `vuhive.http.sse.errors_total` | Counter | `method`, `url` | SSE stream errors, disconnections, or framing failures |
| `vuhive.http.req_connecting` | Duration (HDR) | `method`, `url`, `status` | TCP connection latency *(opt-in via `detailed_timing: true`)* |
| `vuhive.http.req_tls_handshaking` | Duration (HDR) | `method`, `url`, `status` | TLS handshake latency *(opt-in via `detailed_timing: true`)* |
| `vuhive.http.req_sending` | Duration (HDR) | `method`, `url`, `status` | Request payload write latency *(opt-in via `detailed_timing: true`)* |
| `vuhive.http.req_receiving` | Duration (HDR) | `method`, `url`, `status` | Response body read latency *(opt-in via `detailed_timing: true`)* |

### 11.3 Kafka Event Streaming (`pkg/vuhive/kafka`)

For event-driven load tests, use `pkg/vuhive/kafka` (compiled with `-tags kafka`) to produce and consume messages with automatic latency, message count, payload byte, and failure rate tracking.

#### Why Use `pkg/vuhive/kafka`?
- **Zero Standard Dependencies**: Default builds (`!kafka`) link 0 Kafka client code and return `ErrKafkaDisabled`. Compiling with `-tags kafka` activates the high-performance pure-Go driver (`franz-go`).
- **Auto-Recorded Metrics**: Automatically tracks `vuhive.kafka.pub_duration`, `vuhive.kafka.pub_total`, `vuhive.kafka.pub_bytes`, `vuhive.kafka.pub_failed`, `vuhive.kafka.sub_duration`, `vuhive.kafka.sub_total`, `vuhive.kafka.sub_bytes`, and `vuhive.kafka.sub_failed`.
- **Publisher & Consumer Helpers**: `Publish`, `PublishBatch`, `Consume`, `ConsumeBatch`, and `Commit` methods.
- **Enterprise Configuration**: SASL (Plain, SCRAM-256, SCRAM-512), TLS, custom batching, compression (Gzip, Snappy, LZ4, Zstd), and acks.

#### Example Usage

```go
package main

import (
	"fmt"
	"time"

	"github.com/morphy76/vuhive/pkg/vuhive"
	"github.com/morphy76/vuhive/pkg/vuhive/kafka"
)

func main() {
	suite := vuhive.NewSuite("Kafka Event Streaming")

	suite.RegisterScenario("order_stream", vuhive.Scenario{
		Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
			// Initialize Kafka client with broker seeds and consumer group
			client, err := kafka.NewClient(ctx,
				kafka.WithBrokers("localhost:9092"),
				kafka.WithTopic("orders_stream"),
				kafka.WithGroupID("order_validators"),
				kafka.WithTimeout(5*time.Second),
			)
			if err != nil {
				return nil, err
			}
			return map[string]any{"kafka": client}, nil
		},

		RunVU: func(ctx vuhive.VUContext) error {
			client := ctx.GlobalState("kafka").(kafka.Client)

			// 1. Publish message record — auto-records pub_duration, pub_total, pub_bytes, pub_failed
			msg := &kafka.Message{
				Topic: "orders_stream",
				Key:   []byte(fmt.Sprintf("user-%d", ctx.VUID())),
				Value: []byte(`{"event":"order_placed","total":120.00}`),
				Headers: map[string][]byte{"origin": []byte("vuhive")},
			}
			if err := client.Publish(ctx, msg); err != nil {
				return err
			}

			// 2. Consume next message from stream
			record, err := client.Consume(ctx)
			if err != nil {
				return err
			}

			if record != nil {
				// 3. Commit consumed offset
				_ = client.Commit(ctx, record)
			}

			return nil
		},

		Teardown: func(ctx vuhive.TeardownContext, state map[string]any) error {
			if client, ok := state["kafka"].(kafka.Client); ok && client != nil {
				return client.Close()
			}
			return nil
		},
	})

	suite.Execute()
}
```

### 11.4 Data Parameterization (`pkg/vuhive/data`)

Use the dedicated `github.com/morphy76/vuhive/pkg/vuhive/data` package to load CSV, JSON, or JSON Lines datasets with thread-safe record distribution strategies:

- **Loaders**: `data.LoadCSV`, `data.LoadJSON`, `data.LoadJSONL` (or `data.LoadCSVFile`, etc.)
- **Strategies**:
  - `data.Sequential`: Round-robins across records deterministically by VU ID and iteration (requires non-nil context; returns `data.ErrNilContext` if missing).
  - `data.Random`: Lock-free, thread-safe uniform random record selection powered by `math/rand/v2`.
  - `data.UniquePerVU`: Distributes records deterministically per Virtual User ID (requires non-nil context; returns `data.ErrNilContext` if missing).
  - `data.SharedQueue`: Dispenses each record exactly once across concurrent VUs until exhausted (`data.ErrDatasetExhausted`).

```go
Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
    ds, err := data.LoadCSVFile("testdata/users.csv", data.Sequential)
    if err != nil {
        return nil, err
    }
    return map[string]any{"dataset": ds}, nil
},

RunVU: func(ctx vuhive.VUContext) error {
    ds := ctx.GlobalState("dataset").(*data.DataSet)
    record, err := ds.Next(ctx)
    if err != nil {
        return err
    }

    username := record["username"]
    userID := record["user_id"]
    // ... make request with dataset fields ...
    return nil
},
```

### 11.5 Thinking Time & Interaction Delay

Thinking time simulates human reading, processing, or decision delays between user actions, conversation turns, or multi-step requests. Thinking time is **explicitly invoked by the test developer** using `ctx.Sleep()`, which actively respects `ctx.Done()` for immediate cancellation during ramp-down or teardown.

#### Option A: Declarative via `vuhive.yaml`

Configure an `interaction_delay` strategy in `vuhive.yaml`:

```yaml
scenarios:
  conversation_flow:
    type: constant_vus
    vus: 10
    run_period: 2m
    vu_timeout: 5s
    # In-iteration thinking time strategy invoked via ctx.Sleep():
    interaction_delay:
      type: range        # "fixed" | "range" | "expo" | "gaussian"
      min: 200ms
      max: 1s
```

Then call `ctx.Sleep()` at the exact points in your test logic where a user pause occurs (e.g. after receiving a bot response before sending the next message, only if turns remain):

```go
RunVU: func(ctx vuhive.VUContext) error {
    // Action 1: Receive bot response / browse product
    // ...

    // Thinking time before next user interaction (uses configured YAML strategy):
    if err := ctx.Sleep(); err != nil {
        return err // Cancelled due to test termination
    }

    // Action 2: Send next customer message / checkout
    return nil
},
```

#### Option B: Explicit Duration Sleep

Pass an explicit duration to override scenario defaults:

```go
RunVU: func(ctx vuhive.VUContext) error {
    if err := ctx.Sleep(300 * time.Millisecond); err != nil {
        return err
    }
    return nil
},
```

#### Option C: Custom Mathematical Delay Generators

`vuhive` provides 4 mathematical distribution generators:
- `vuhive.FixedDelay(d)` — constant duration
- `vuhive.RangeDelay(min, max)` — uniform random $U(\text{min}, \text{max})$
- `vuhive.ExpoDelay(mean, min, max)` — exponential distribution (Poisson arrival) with optional clamping
- `vuhive.GaussianDelay(mean, stdDev, min, max)` — normal distribution $N(\mu, \sigma)$ with non-negative guarantee and optional clamping

```go
thinkTimer := vuhive.ExpoDelay(500*time.Millisecond, 100*time.Millisecond, 2*time.Second)

RunVU: func(ctx vuhive.VUContext) error {
    if err := ctx.Sleep(thinkTimer.Next()); err != nil {
        return err
    }
    return nil
},
```



### 11.6 Multi-Step User Journey

```go
RunVU: func(ctx vuhive.VUContext) error {
    // Step 1: Login
    token, err := login(ctx, baseURL, username, password)
    if err != nil {
        return fmt.Errorf("login failed: %w", err)
    }

    // Step 2: Browse catalog
    products, err := browseCatalog(ctx, baseURL, token)
    if err != nil {
        return fmt.Errorf("browse failed: %w", err)
    }

    // Step 3: Add to cart
    if err := addToCart(ctx, baseURL, token, products[0].ID); err != nil {
        return fmt.Errorf("add to cart failed: %w", err)
    }

    // Step 4: Checkout
    if err := checkout(ctx, baseURL, token); err != nil {
        return fmt.Errorf("checkout failed: %w", err)
    }

    return nil
},
```

### 11.7 Conditional Metric Recording with Tags

```go
if resp.StatusCode == http.StatusOK {
    ctx.Metrics().Counter("http_2xx", vuhive.Tags{}).Inc()
} else if resp.StatusCode >= 500 {
    ctx.Metrics().Counter("http_5xx", vuhive.Tags{}).Inc()
}
```

### 11.8 Execution Summary Hook (Slack Webhooks & Custom Metrics)

Use `HandleSummary` to receive the full structured summary after report generation for notifications, metrics export, or CI artifact generation:

```go
HandleSummary: func(ctx vuhive.SummaryContext, summary vuhive.SummaryData) error {
    // 1. Check overall SLA pass/fail status
    if !summary.Passed {
        postSlackAlert(fmt.Sprintf("❌ Load test SLA breached for %s (%s)", summary.Scenario, summary.SuiteName))
    }

    // 2. Read specific metric aggregates
    totalRequests := summary.Counter("http_requests_total")
    successRate := summary.Rate("success_rate")
    latency := summary.Metric("http_request_duration")

    if latency != nil {
        fmt.Printf("Summary: %d reqs, success rate: %.2f%%, p95 latency: %v\n",
            totalRequests, successRate*100, latency.P95)
    }

    // 3. Export custom JSON document
    jsonBytes, err := summary.JSON()
    if err == nil {
        _ = os.WriteFile("ci-summary.json", jsonBytes, 0644)
    }

    return nil
},
```

### 11.9 Inline Assertions (Checks)

```go
RunVU: func(ctx vuhive.VUContext) error {
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    // Assert HTTP status 200 without aborting iteration on check fail
    ctx.Check("status is 200", func() string {
        if resp.StatusCode != 200 {
            return fmt.Sprintf("expected 200, got %d", resp.StatusCode)
        }
        return ""
    })

    return nil
}
```

---


## 12. Troubleshooting

### "vuhive: RegisterScenario called with nil RunVU"

**Cause**: `Scenario.RunVU` is nil.  
**Fix**: Every scenario must have a non-nil `RunVU` function.

### VU iterations all timing out

**Cause**: `vu_timeout` is too short for your workload.  
**Fix**: Increase `vu_timeout` in `vuhive.yaml`. For SSE/WebSocket scenarios, use a longer timeout or implement per-event timeouts in your RunVU logic.

### "metric X already registered as Counter"

**Cause**: You used the same metric name for two different types.  
**Fix**: Each metric name is locked to its first-registered type. Use distinct names.

### No iterations completing (0 total)

**Cause**: `run_period` is shorter than one RunVU iteration.  
**Fix**: Increase `run_period` or add `ramp_down` to give in-flight iterations a grace period.

### Arrival rate: many dropped iterations

**Cause**: `max_vus` is too low for `target_tps` given your iteration latency.  
**Formula**: `max_vus ≥ target_tps × avg_iteration_latency_seconds`  
**Fix**: Increase `max_vus` or optimize your RunVU to reduce latency.

---

## 13. Reference Implementations (`examples/`)

The repository includes a comprehensive set of compilable, self-contained example suites under [`examples/`](../examples/README.md). Each example is paired with an in-process mock server (`httptest.Server` or simulated domain model), a companion `vuhive.yaml` configuration, and a dedicated `README.md` reference guide.

See the [**Examples Reference Suite Index**](../examples/README.md) for a structured 3-tier learning path and full capability matrix.

| Example | Directory | Core Concepts | Guide |
|---|---|---|---|
| **HTTP REST Checkout** | [`examples/http_checkout/`](../examples/http_checkout/) | Standard REST API load test, HDR duration histogram, success rate, constant VU concurrency. | [Read Guide](../examples/http_checkout/README.md) |
| **HTTP Module (Auto-Metrics)** | [`examples/http_module/`](../examples/http_module/) | Built-in `pkg/vuhive/http` client, automatic metrics, JSON parsing, inline checks. | [Read Guide](../examples/http_module/README.md) |
| **Inline Checks (Assertions)** | [`examples/checks/`](../examples/checks/) | Inline assertions (`ctx.Check`), non-aborting validations, auto-instrumented check counters and report tables. | [Read Guide](../examples/checks/README.md) |
| **Transaction Groups** | [`examples/groups/`](../examples/groups/) | Transaction boundaries (`ctx.Group`), nested groups, group-scoped duration metrics, threshold targeting. | [Read Guide](../examples/groups/README.md) |
| **Thinking Time & Delays** | [`examples/think_time/`](../examples/think_time/) | Multi-step journey, declarative `interaction_delay` (`range`), `ctx.Sleep()`, programmatic `ExpoDelay`. | [Read Guide](../examples/think_time/README.md) |
| **Data Parameterization** | [`examples/data_parameterization/`](../examples/data_parameterization/) | `pkg/vuhive/data` dataset ingestion (CSV, JSON, JSONL) with `Sequential`, `Random`, and `SharedQueue` strategies. | [Read Guide](../examples/data_parameterization/README.md) |
| **Ramping VUs Spike Test** | [`examples/ramping_vus/`](../examples/ramping_vus/) | Multi-stage spike test with dynamic VU scaling and recovery observation (`ramping_vus`). | [Read Guide](../examples/ramping_vus/README.md) |
| **SLA Thresholds & Quality Gates** | [`examples/sla_thresholds/`](../examples/sla_thresholds/) | Multi-metric thresholds, percentile gates, rate assertions, and early test termination with `abort_on_fail`. | [Read Guide](../examples/sla_thresholds/README.md) |
| **Execution Summary Hook** | [`examples/handle_summary/`](../examples/handle_summary/) | `HandleSummary` lifecycle hook, summary data inspection, programmatic webhook dispatch and artifact generation. | [Read Guide](../examples/handle_summary/README.md) |
| **Conversational AI Flow** | [`examples/conversation_flow/`](../examples/conversation_flow/) | Real-time Server-Sent Events (SSE) streaming, multi-turn state machine, DSL client architecture. | [Read Guide](../examples/conversation_flow/README.md) |
| **Server-Sent Events (SSE)** | [`examples/sse_streaming/`](../examples/sse_streaming/) | Built-in `pkg/vuhive/http` Server-Sent Events (SSE) streaming, TTFE latency, token throughput. | [Read Guide](../examples/sse_streaming/README.md) |
| **gRPC RPC Service** | [`examples/grpc_user_service/`](../examples/grpc_user_service/) | Open-system arrival rate pacing (`arrival_rate`), target TPS, bounded worker pool (`max_vus`). | [Read Guide](../examples/grpc_user_service/README.md) |
| **Kafka Event Streaming** | [`examples/kafka/`](../examples/kafka/) | High-throughput Kafka Publisher & Consumer (`pkg/vuhive/kafka`), conditional compilation with `-tags kafka`. | [Read Guide](../examples/kafka/README.md) |

### Running Examples

Run any example directly by navigating to its directory or passing the config path:

```bash
# Run any example directly from repo root:
go run -tags=vuhive_example ./examples/think_time --config ./examples/think_time/vuhive.yaml

# Or navigate to the directory:
cd examples/think_time
go run -tags=vuhive_example .
```

Verify that all examples compile cleanly across the entire workspace:

```bash
make test-examples
```

---

## 14. Performance & Framework Overhead Optimization

When load testing high-throughput systems (generating 50,000–500,000+ transactions per second), framework execution overhead must be as close to zero as possible. Any CPU cycles spent in framework machinery delay request dispatch and distort latency histograms (coordinated omission).

`vuhive` incorporates extensive low-level optimizations across context lifecycle, metric ingestion, and pacing concurrency.

### 14.1 Zero-Allocation Virtual User Execution Loop

In steady-state execution, each Virtual User goroutine runs with **0 heap allocations per iteration**:

1. **Context Reuse**: The `ScenarioContext` instance is allocated once when the VU starts and is reused across all subsequent iterations. Only the iteration counter and active context are updated in place.
2. **Hoisted Configurations**: Static scenario configurations (e.g. `interaction_delay` / `think_time` generators) are initialized once per VU rather than re-instantiated per iteration.
3. **Pre-Bound Logging Context**: VU ID and scenario names are bound to the logger once at VU startup, avoiding repetitive zerolog dictionary allocations during iterations.
4. **Lightweight Timeout Management**: When `vu_timeout` is not set or handled at the protocol layer, per-iteration Go runtime timer registration (`context.WithTimeout`) is bypassed entirely.

### 14.2 Lock-Free Metrics Architecture

Telemetry collection is built for ultra-high concurrency across multi-core processors:

| Mechanism | Implementation | Benefit |
|---|---|---|
| **Copy-on-Write Storage** | `atomic.Pointer[map[metricKey]V]` | Read lookups (`ctx.Metrics().Counter(...)`) are 100% lock-free, eliminating CPU cache-line bouncing across multi-core systems. |
| **Sharded HDR Histograms** | 16-stripe mutex-guarded shards | `ctx.Metrics().Duration(...).Observe(d)` distributes observations across 16 shards, eliminating global mutex contention. |
| **Fast Tag Key Generation** | Slice-free formatting | Empty tags (`Tags{}`) and single-tag lookups (`Tags{"name": "check"}`) allocate zero slices and skip string sorting. |
| **Pre-Resolved Check Handles** | Internal `checkCounterPair` cache | Inline assertions (`ctx.Check`) resolve metric handles once and evaluate in **~6.4ns** with zero heap allocations. |

### 14.3 Bounded Worker Pool Pacing (`arrival_rate`)

In `arrival_rate` mode, `vuhive` maintains a pre-allocated worker pool of up to `max_vus` persistent goroutines consuming iteration jobs from a bounded channel. This eliminates continuous goroutine creation and destruction under high TPS, preventing runtime scheduler thrashing and GC metadata overhead.

### 14.4 Running Microbenchmarks

`vuhive` includes an extensive microbenchmark suite (`testing.B` with `-benchmem`) covering all engine pacing modes, context operations, and metric collection handles.

Execute all microbenchmarks:

```bash
make test-bench
```

Expected baseline performance on modern hardware (e.g. Apple Silicon / Linux x86-64):

| Benchmark Target | Latency / Throughput | Memory Allocation |
|---|---|---|
| `BenchmarkScenarioContext_Check` | **~6.4 ns/op** | **0 B/op (0 allocs/op)** |
| `BenchmarkCollector_Counter_Parallel` | **~65 ns/op** | **0 B/op (0 allocs/op)** |
| `BenchmarkCollector_Gauge_Parallel` | **~55 ns/op** | **0 B/op (0 allocs/op)** |
| `BenchmarkCollector_Duration_Parallel` | **~115 ns/op** | **0 B/op (0 allocs/op)** |
| `BenchmarkEngine_ConstantVUs_NoopIteration` | **> 1,000,000 iter/s** | **0 allocs/op steady-state** |

### 14.5 Go Runtime & Host System Tuning for Maximum Load Generation

When executing high-throughput load tests (e.g. 50k–500k+ TPS or thousands of concurrent VUs), tuning Go runtime environment variables and host OS network limits ensures that the load generator runs at maximum efficiency without self-inflicted bottlenecks.

#### A. Go Runtime Environment Variables

| Variable | Recommended Value | Purpose & Performance Impact |
|---|---|---|
| **`GOMEMLIMIT`** | `80-90%` of container/host RAM (e.g. `GOMEMLIMIT=7GiB` on an 8GB host) | Sets a soft memory ceiling for the Go GC. Enables the runtime to utilize available RAM efficiently and avoid premature GC cycles during high-throughput load tests. |
| **`GOGC`** | `200` to `500` (or `off` when paired with `GOMEMLIMIT`) | Controls garbage collection frequency relative to heap size. Higher values delay GC cycles during test execution, saving CPU cycles and eliminating GC latency jitter. Paired with `GOMEMLIMIT`, `GOGC=300` or `GOGC=500` ensures GC only triggers when approaching memory boundaries. |
| **`GOMAXPROCS`** | Equal to available physical/container CPU cores (e.g. `GOMAXPROCS=16`) | Sets the number of operating system threads executing Go code concurrently. In containerized environments (Kubernetes/Docker), ensure `GOMAXPROCS` matches container CPU limits to prevent OS CPU throttling. |
| **`GODEBUG`** | `gctrace=1` (for diagnostic dry-runs) | Emits GC activity statistics to stderr on every GC cycle (heap size, mark duration, pause time). Use during test dry-runs to verify that GC pauses remain sub-millisecond. |

Example command running a tuned high-load test:

```bash
GOMEMLIMIT=7GiB GOGC=300 GOMAXPROCS=16 ./my-load-test --config vuhive.yaml --scenario high_tps_checkout
```

#### B. Client Connection & HTTP Transport Tuning

When load testing HTTP/REST endpoints with high concurrency, default Go `http.Transport` settings can throttle client throughput:

```go
Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
    // Tune HTTP transport for high concurrency load generation
    transport := &http.Transport{
        MaxIdleConns:        10000,
        MaxIdleConnsPerHost: 5000,
        MaxConnsPerHost:     0, // unlimited
        IdleConnTimeout:     90 * time.Second,
        DisableKeepAlives:   false, // reuse TCP connections
        ForceAttemptHTTP2:   true,
    }
    client := &http.Client{
        Transport: transport,
        Timeout:   5 * time.Second,
    }
    return map[string]any{"client": client}, nil
}
```

- **`MaxIdleConnsPerHost`**: The standard library default is `2`, which causes massive TCP socket churning and connection re-establishment (`TIME_WAIT` socket buildup). Increase to `1000–5000+` for high-throughput load generation.
- **`DisableKeepAlives: false`**: Enables HTTP keep-alive connection reuse, eliminating TCP 3-way handshake and TLS negotiation overhead on every iteration.

#### C. Operating System & Network Socket Limits

Load generator nodes creating thousands of concurrent connections require adequate OS file descriptor and network socket limits:

```bash
# 1. Increase file descriptor limits (prevent "too many open files")
ulimit -n 65536

# 2. Expand ephemeral port range (Linux sysctl)
sysctl -w net.ipv4.ip_local_port_range="1024 65535"

# 3. Enable fast TCP connection reuse for TIME_WAIT sockets (Linux sysctl)
sysctl -w net.ipv4.tcp_tw_reuse=1

# 4. Increase socket listen backlog and FIN timeout (Linux sysctl)
sysctl -w net.core.somaxconn=65535
sysctl -w net.ipv4.tcp_fin_timeout=15
```


