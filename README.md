# vuhive — High-Performance Go Load Testing Framework

`vuhive` is a developer-centric, high-performance load testing library and execution framework built for Go 1.26+. It separates load profile configuration from scenario execution code, allowing developers to define test scenarios in Go with rich lifecycle hooks while managing concurrency, pacing profiles, and SLA assertions declaratively via YAML.

> **New to vuhive?** See the [Developer Guide](docs/GUIDE.md) for a step-by-step adoption walkthrough.

---

## Key Features

- **Hexagonal Architecture with DDD Boundaries**: Clean separation of core domain models (`pkg/vuhive`), configuration engines, pacing engines, metrics storage, and reporting CLI adapters.
- **Triple Pacing Engines & Zero-Allocation Hot Path**:
  - **`constant_vus`**: Closed-system model maintaining a fixed number of Concurrent Virtual Users with reusable per-VU execution contexts and zero steady-state heap allocations.
  - **`arrival_rate`**: Open-system token bucket rate-limiting engine (`golang.org/x/time/rate`) targeting precise Transactions Per Second (TPS) with a pre-allocated bounded worker pool (`max_vus`) eliminating per-iteration goroutine churning.
  - **`ramping_vus`**: Dynamic multi-stage pacing engine allowing stage-based VU target ramps, holds, and spikes over time.
- **Lock-Free In-Memory Metrics Engine**: Atomic counters, CAS gauges, atomic rate tracking, copy-on-write atomic pointer map storage, and 16-stripe sharded HDR Histograms (`github.com/HdrHistogram/hdrhistogram-go`) providing zero-contention, high-resolution percentile calculations (`p50`, `p90`, `p95`, `p99`, `mean`, `min`, `max`).
- **Structured Logging**: Zerolog (`github.com/rs/zerolog`) integration with hoisted VU ID and scenario context bindings.
- **Transaction Boundaries (Groups)**: Organize `RunVU` logic into named transaction steps and nested sub-groups with `ctx.Group(name, fn)`. Automatically measures per-step latency (`vuhive.group.<path>.duration`), formats dedicated `GROUPS` summary tables, and enables granular per-step SLA quality gates.
- **Instrumented HTTP Client Module (`pkg/vuhive/http`)**: High-performance HTTP client helper (`vuhivehttp.Default`, `vuhivehttp.NewClientFromConfig`, `vuhivehttp.NewClient`) with declarative YAML configuration (`vuhive.yaml`), automatic metric collection (HDR request duration histograms, request counters, failure rates), response body parsing helpers (`.JSON()`, `.Text()`), opt-in `httptrace` phase latency breakdowns, connection pool tuning, and first-class **Server-Sent Events (SSE)** response streaming (`client.StreamSSE`, `client.DoStream`, `*vuhivehttp.SSEStream`) with automatic **iteration deadline detachment** for long-lived streams, context precedence resolution, and dedicated real-time streaming telemetry.
- **Kafka Messaging Module (`pkg/vuhive/kafka`)**: Auto-instrumented Kafka Publisher and Consumer clients conditionally compiled via Go build tags (`-tags kafka`) for testing event-driven architectures with zero dependencies in standard builds.
- **Data Parameterization Module (`pkg/vuhive/data`)**: CSV, JSON, and JSON Lines dataset loaders (`LoadCSV`, `LoadJSON`, `LoadJSONL`) supporting thread-safe distribution strategies (`Sequential`, `Random`, `UniquePerVU`, `SharedQueue`).
- **SLA Threshold Evaluator & Graceful Abort**: Declarative quality gates evaluated post-execution, with optional real-time early termination (`abort_on_fail: true`, `delay_abort_eval: 5s`) to stop runaway failures instantly. Returns exit code `0` on success or `1` on SLA breach/abort.
- **Deterministic Reporting**: Terminal summary and JSON reports (§10 schema) with alphabetically sorted metrics.

---

## Installation

```bash
go get github.com/morphy76/vuhive
```

---

## Quick Start (Single Scenario)

For focused benchmarks and single-scenario tests, `vuhive.Run` eliminates boilerplate ceremonies by executing the scenario with CLI arguments and calling `os.Exit` with the appropriate exit code:

```go
package main

import (
	"github.com/morphy76/vuhive/pkg/vuhive"
	vuhivehttp "github.com/morphy76/vuhive/pkg/vuhive/http"
)

func main() {
	vuhive.Run("http_checkout", func(ctx vuhive.VUContext) error {
		_, err := vuhivehttp.Default(ctx).Get(ctx, "/checkout")
		return err
	})
}
```

Optional lifecycle hooks can be passed via functional options (`WithSetup`, `WithPreTest`, `WithAfterTest`, `WithTeardown`, `WithSummary`):

```go
func main() {
	vuhive.Run("http_checkout",
		func(ctx vuhive.VUContext) error {
			client := ctx.GlobalState("client").(*http.Client)
			// ...
			return nil
		},
		vuhive.WithSetup(func(ctx vuhive.SetupContext) (map[string]any, error) {
			return map[string]any{"client": &http.Client{}}, nil
		}),
		vuhive.WithTeardown(func(ctx vuhive.TeardownContext, state map[string]any) error {
			return nil
		}),
	)
}
```

---

## Core Facilities & Lifecycle Hooks

For complex load testing with multiple registered scenarios, create a suite using `vuhive.NewSuite("Suite Name")`. Each scenario registers up to 6 lifecycle hooks:

```go
type Scenario struct {
    Setup          func(ctx SetupContext) (map[string]any, error)
    PreTest        func(ctx VUContext) error
    RunVU          func(ctx VUContext) error
    AfterTest      func(ctx VUContext) error
    Teardown       func(ctx TeardownContext, state map[string]any) error
    HandleSummary  func(ctx SummaryContext, summary SummaryData) error
}
```

### Lifecycle Hook Sequence

```text
       ┌────────────────────────────┐
       │   Setup(ctx SetupContext)  │  (Runs once per scenario before VUs spawn)
       └─────────────┬──────────────┘
                     │  returns globalState map[string]any
                     ▼
┌──────────────────────────────────────────────┐
│ For each VU Iteration:                       │
│                                              │
│   ┌────────────────────────┐                 │
│   │   PreTest(ctx VUContext│                 │
│   └───────────┬────────────┘                 │
│               │ (if err != nil, skips RunVU) │
│               ▼                              │
│   ┌────────────────────────┐                 │
│   │   RunVU(ctx VUContext) │                 │
│   └───────────┬────────────┘                 │
│               │                              │
│               ▼                              │
│   ┌────────────────────────┐                 │
│   │  AfterTest(ctx VUContxt│ (defer guarantee│
│   └────────────────────────┘  runs always)   │
└──────────────────┬───────────────────────────┘
                   │
                   ▼
       ┌──────────────────────────────────────┐
       │ Teardown(ctx TeardownContext, state) │  (Runs once after all VUs exit)
       └───────────┬──────────────────────────┘
                   │
                   ▼
       ┌──────────────────────────────────────┐
       │ HandleSummary(ctx SummaryCtx, summ.) │  (Runs post-report with full execution summary)
       └──────────────────────────────────────┘
```


---

## Context Hierarchy & Capabilities

Adhering to the **Interface Segregation Principle (ISP)**, vuhive provides role-specific context interfaces (`SetupContext`, `VUContext`, `TeardownContext`, `SummaryContext`) composing granular capability interfaces. `ScenarioContext` is preserved as an alias to `VUContext` for backward compatibility.

| Method | Capability Interface | Description |
|--------|----------------------|-------------|
| `ctx.VUID()` | `ExecutionIdentity` | Returns the 1-based Virtual User ID (`int64`). |
| `ctx.Iteration()` | `ExecutionIdentity` | Returns the 0-based iteration index (`int64`). |
| `ctx.ScenarioName()` | `ExecutionIdentity` | Returns the scenario string identifier. |
| `ctx.Param(key)` | `ConfigProvider` | Returns scenario param string from YAML config. |
| `ctx.ParamInt(key, default)` | `ConfigProvider` | Parses scenario param as integer (logs warning and returns default on parse failure). |
| `ctx.ParamDuration(key, default)` | `ConfigProvider` | Parses scenario param as `time.Duration` (e.g. `200ms`, logs warning and returns default on parse failure). |
| `ctx.HTTPConfig()` | `ConfigProvider` | Returns typed declarative HTTP client configuration (BaseURL, Timeout, Headers, TLS, Pool). |
| `ctx.GlobalState(key)` | `StateProvider` | Accesses values returned by the `Setup` hook (shallow-copied, read-only). |
| `ctx.Log()` | `ObservabilityProvider` | Structured `Logger` instance bound with VU ID and iteration context. |
| `ctx.Metrics()` | `ObservabilityProvider` | `MetricsCollector` for recording custom counters, gauges, durations, and rates. |
| `ctx.Sleep(d ...time.Duration)` | `WorkflowController` | Pauses for explicit duration or configured `interaction_delay` strategy (respects `ctx.Done()`). |
| `ctx.Check(name, fn)` | `WorkflowController` | Evaluates inline pass/fail assertion (`CheckFunc`) without stopping VU iteration execution. |
| `ctx.Group(name, fn)` | `WorkflowController` | Executes `fn` within a named transaction boundary with automatic latency recording (`vuhive.group.<path>.duration`). |



---

## Inline Assertions (Checks)

Inline assertions allow developers to validate real-time pass/fail conditions inside `RunVU` without terminating the iteration:

```go
ctx.Check("status code is 200", func() string {
    if resp.StatusCode != http.StatusOK {
        return fmt.Sprintf("expected 200, got %d", resp.StatusCode)
    }
    return "" // empty string indicates check passed
})
```

- **Pass/Fail Contract**: Return `""` (empty string) for pass (`true`); return non-empty failure reason string for fail (`false`).
- **Auto-Instrumentation**: Automatically increments built-in counters `vuhive.checks.passed` and `vuhive.checks.failed` tagged with `name`.
- **Reporting & Thresholds**: Per-check pass/fail counts and percentages are displayed in console and JSON reports. SLA thresholds can target check metrics (e.g. `vuhive.checks.failed count == 0`).

---

## Groups (Transaction Boundaries)

Groups organize multi-step Virtual User flows into named transaction boundaries with automatic latency measurement (inspired by k6 `group()` and Gatling `exec().group()`):

```go
err := ctx.Group("01_Login", func(ctx vuhive.VUContext) error {
    resp, err := client.Post(baseURL+"/api/login", "application/json", body)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    return nil
})
```

### Nested Groups

Groups can be nested to model sub-transactions. Child group metric names are concatenated with the `::` delimiter:

```go
err := ctx.Group("03_Checkout", func(ctx vuhive.VUContext) error {
    // Nested group 1: Add to cart
    if err := ctx.Group("Add_To_Cart", func(ctx vuhive.VUContext) error {
        return addItemToCart(ctx)
    }); err != nil {
        return err
    }

    // Nested group 2: Submit payment
    return ctx.Group("Submit_Payment", func(ctx vuhive.VUContext) error {
        return submitPayment(ctx)
    })
})
```

- **Automatic Metrics**: Each group records an HDR duration histogram named `vuhive.group.<path>.duration` (e.g. `vuhive.group.03_Checkout::Submit_Payment.duration`).
- **Reporting**: Formatted in a dedicated `GROUPS` summary table in the terminal report and a `groups` array in JSON output.
- **SLA Thresholds**: Set latency targets directly on group metrics in `vuhive.yaml`:
  ```yaml
  thresholds:
    - metric: "vuhive.group.01_Login.duration"
      stat: p95
      operator: "<"
      target: "200ms"
    - metric: "vuhive.group.03_Checkout::Submit_Payment.duration"
      stat: p95
      operator: "<"
      target: "250ms"
  ```

---

## Recording Metrics


`vuhive` provides four metric types accessed via `ctx.Metrics()`:

```go
// 1. Duration (HDR Histogram)
ctx.Metrics().Duration("http_request_duration", vuhive.Tags{"endpoint": "/checkout"}).Observe(120 * time.Millisecond)

// 2. Counter
ctx.Metrics().Counter("http_requests_total", vuhive.Tags{"status": "200"}).Inc()
ctx.Metrics().Counter("bytes_transferred", vuhive.Tags{}).Add(1024)

// 3. Gauge
ctx.Metrics().Gauge("active_connections", vuhive.Tags{}).Set(42)

// 4. Rate (Explicit Numerator/Denominator)
ctx.Metrics().Rate("checkout_success_rate", vuhive.Tags{}).Add(1, 1) // 1 success out of 1 trial
```

Built-in framework metrics (`vuhive.MetricIterationsTotal`, `vuhive.MetricChecksPassed`, etc.) are pre-registered and exported as constants in package `vuhive`. See [Developer Guide](docs/GUIDE.md#built-in-metrics-auto-recorded-by-the-framework) for the full inventory.

---

## Configuration (`vuhive.yaml`)

Configuration is managed declaratively in `vuhive.yaml`. Add `# yaml-language-server: $schema=...` at the top of your configuration file for out-of-the-box IDE autocompletion and validation (see [IDE Integration Guide](docs/GUIDE.md#ide-autocompletion--schema-validation)):

### Example `vuhive.yaml`

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/morphy76/vuhive/main/schemas/vuhive.schema.json
version: "1.0"
default_scenario: http_checkout_flow

scenarios:
  http_checkout_flow:
    type: constant_vus
    vus: 10
    ramp_up: 5s
    run_period: 30s
    ramp_down: 5s
    vu_timeout: 2s
    http:
      base_url: "https://api.example.com"
      timeout: 5s
      headers:
        Accept: "application/json"
      pool:
        max_idle_conns: 100
        max_idle_conns_per_host: 10
    params:
      checkout_path: "/api/checkout"
    thresholds:
      - metric: http_request_duration
        stat: p95
        operator: "<"
        target: "200ms"
      - metric: checkout_success_rate
        stat: rate
        operator: ">="
        target: "0.99"
      - metric: http_requests_total
        stat: count
        operator: ">="
        target: "500"

  user_registration_api:
    type: arrival_rate
    target_tps: 50
    max_vus: 20
    ramp_up: 10s
    run_period: 1m
    vu_timeout: 1s
    thresholds:
      - metric: vuhive.pacing.dropped_iterations
        stat: count
        operator: "<="
        target: "0"
```

### Supported Pacing Modes

1. **`constant_vus`**:
   - `vus`: Number of concurrent VUs (`int > 0`).
   - `ramp_up`: Staggered linear VU spawn duration (`time.Duration`).
   - `run_period`: Steady-state load duration (`time.Duration`).
   - `ramp_down`: Graceful exit duration for active iteration dispatch (`time.Duration`, default `0s`).
   - `drain`: Grace period for in-flight VUs to complete before cancellation (`time.Duration`, default `0s`). Alias: `drain_period`.

2. **`arrival_rate`**:
   - `target_tps`: Desired transactions/iterations per second (`int > 0`).
   - `max_vus`: Maximum size of the worker pool (`int > 0`). If the pool saturates, unhandled tokens increment `vuhive.pacing.dropped_iterations`.
   - `ramp_up`: Linear rate ramp-up duration (`time.Duration`).
   - `run_period`: Steady-state arrival duration (`time.Duration`).
   - `ramp_down`: Duration for active token dispatch ramp-down (`time.Duration`, default `0s`).
   - `drain`: Grace period for in-flight workers to complete before cancellation (`time.Duration`, default `0s`). Alias: `drain_period`.

3. **`ramping_vus`**:
   - `stages`: List of stage definitions (`target: int`, `duration: time.Duration`).
   - `ramp_down`: Duration for stage ramp-down (`time.Duration`, default `0s`).
   - `drain`: Grace period for in-flight workers to complete before cancellation (`time.Duration`, default `0s`). Alias: `drain_period`.
   - `vu_timeout`: Per-iteration timeout (`time.Duration`).
   - *Details and patterns in [Developer Guide](docs/GUIDE.md#ramping_vus-multi-stage-pacing).*


---

## Thinking Time & Interaction Delay Strategies

Simulate realistic human reading and decision pauses between user actions, conversation turns, or multi-step requests. Thinking time is **explicitly invoked by the test developer** using `ctx.Sleep()` and configured declaratively in `vuhive.yaml` (or generated programmatically).

### Supported Delay Strategies

| Strategy | YAML Type | Key Parameters | Description |
|---|---|---|---|
| **Fixed** | `fixed` | `duration: 500ms` | Static deterministic pause. |
| **Range** | `range` | `min: 200ms`, `max: 1s` | Uniform random distribution $U(\text{min}, \text{max})$. |
| **Exponential** | `expo` | `mean: 500ms`, `min`, `max` (optional) | Exponential distribution (Poisson arrival modeling) $D = -\text{mean} \cdot \ln(U)$ with optional clamping. |
| **Gaussian** | `gaussian` | `mean: 500ms`, `std_dev: 100ms`, `min`, `max` (optional) | Normal distribution $N(\mu, \sigma)$ with non-negative guarantee and optional clamping. |

### Configuration in `vuhive.yaml`

```yaml
scenarios:
  user_checkout:
    type: constant_vus
    vus: 10
    run_period: 1m
    vu_timeout: 5s
    # Thinking time strategy used when calling ctx.Sleep() without arguments:
    interaction_delay:
      type: range
      min: 200ms
      max: 1s
```

### Usage in Code

```go
RunVU: func(ctx vuhive.VUContext) error {
    // Step 1: Browse catalog / receive message
    // ...

    // Explicitly execute thinking time using scenario-configured strategy (respects ctx.Done())
    if err := ctx.Sleep(); err != nil {
        return err // aborted due to context cancellation
    }

    // Or pause for an explicit duration
    if err := ctx.Sleep(250 * time.Millisecond); err != nil {
        return err
    }

    // Step 2: Next action (e.g. add to cart / next customer message)
    return nil
}
```

Programmatic generators are also available: `vuhive.FixedDelay(d)`, `vuhive.RangeDelay(min, max)`, `vuhive.ExpoDelay(mean, min, max)`, `vuhive.GaussianDelay(mean, stdDev, min, max)`.

---

## Data Parameterization (`pkg/vuhive/data`)

Feed external datasets into your load tests using the built-in `pkg/vuhive/data` module supporting CSV, JSON, and JSON Lines formats with thread-safe row selection strategies:

### Supported Dataset Formats & Loaders

| Format | Loader Function | Description |
|---|---|---|
| **CSV** | `data.LoadCSV(reader, strategy)` / `data.LoadCSVFile(path, strategy)` | Reads CSV with headers into string key-value maps (`Record`). |
| **JSON** | `data.LoadJSON(reader, strategy)` / `data.LoadJSONFile(path, strategy)` | Parses a JSON array of objects into key-value records. |
| **JSONL** | `data.LoadJSONL(reader, strategy)` / `data.LoadJSONLFile(path, strategy)` | Parses newline-delimited JSON objects line-by-line. |

### Distribution Strategies

| Strategy | Enum Constant | Behavior |
|---|---|---|
| **Sequential** | `data.Sequential` | Deterministic round-robin across rows based on VU ID and iteration index: `(vuid - 1 + iteration) % N` (requires context). |
| **Random** | `data.Random` | Lock-free, thread-safe uniform random selection across all rows. |
| **UniquePerVU** | `data.UniquePerVU` | Partitions rows to avoid VU overlap where possible (requires context). |
| **SharedQueue** | `data.SharedQueue` | Thread-safe atomic single-consumption queue. Returns `data.ErrDatasetExhausted` when depleted. |

### Example Usage

```go
Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
    ds, err := data.LoadCSVFile("data/users.csv", data.Sequential)
    if err != nil {
        return nil, err
    }
    return map[string]any{"users": ds}, nil
},
RunVU: func(ctx vuhive.VUContext) error {
    ds := ctx.GlobalState("users").(*data.DataSet)
    user, err := ds.Next(ctx)
    if err != nil {
        return err
    }
    // Access string values by column/key name
    userID := user["user_id"]
    username := user["username"]
    // ...
    return nil
},
```

---

## Instrumented HTTP Module (`pkg/vuhive/http`)

Eliminate repetitive boilerplate metric instrumentation from your `RunVU` loops by using the built-in `pkg/vuhive/http` package. It wraps standard HTTP request execution with automatic latency tracking, status code tagging, failure rate calculation, and convenient response decoding helpers.

### Advantages Over Raw `http.Client`

- **Zero-Boilerplate Metrics**: Latency duration histograms (`vuhive.http.req_duration`), request counts (`vuhive.http.reqs`), and failure rates (`vuhive.http.req_failed`) are recorded automatically for every request with method, path, and status code tags.
- **Declarative YAML Configuration**: Configure base URL, timeouts, default headers, connection pool parameters, and TLS options directly in `vuhive.yaml` under `scenarios.<name>.http`.
- **Convenient Response Parsing**: `Response.JSON(target)` unmarshals directly into Go structs, and `Response.Text()` extracts raw body strings while eagerly closing the underlying response body.
- **Configurable Connection Pooling & Defaults**: Easily configure timeouts, default authorization headers, connection pool bounds, and TLS skip verification declaratively in YAML or programmatically using fluent functional options.
- **Opt-in Phase-Breakdown Timing**: Trace TCP connection time, TLS handshake duration, request write time, and response read time via `httptrace` (`detailed_timing: true` in YAML or `vuhivehttp.WithDetailedTiming()`).
- **Server-Sent Events (SSE) Streaming**: First-class support for persistent `text/event-stream` connections (`client.StreamSSE`, `client.DoStream`). Stream events iteratively (`stream.Next()`, `stream.Event()`) or via channels (`stream.Events()`) with zero unbounded memory buffering and dedicated real-time streaming metrics (TTFE latency, token throughput, stream duration). Automatically **detaches short parent iteration deadlines** (`vu_timeout`) so long-lived SSE streams survive beyond per-iteration timeouts while remaining responsive to explicit context cancellation (scenario teardown, VU shutdown).

### Declarative Configuration (`vuhive.yaml`)

```yaml
scenarios:
  http_checkout:
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

### Basic Usage

```go
package main

import (
	"fmt"
	"net/http"

	"github.com/morphy76/vuhive/pkg/vuhive"
	vuhivehttp "github.com/morphy76/vuhive/pkg/vuhive/http"
)

type CheckoutResult struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

func main() {
	suite := vuhive.NewSuite("Checkout API Load Test")

	suite.RegisterScenario("http_checkout", vuhive.Scenario{
		// No Setup hook needed — vuhivehttp.Default(ctx) lazily retrieves the shared client
		RunVU: func(ctx vuhive.VUContext) error {
			// Retrieve scenario's shared HTTP client initialized from vuhive.yaml
			client := vuhivehttp.Default(ctx)

			// Execute request — relative URL resolved against BaseURL, metrics auto-recorded
			resp, err := client.Get(ctx, "/api/checkout")
			if err != nil {
				return err
			}

			// Validate with inline checks
			ctx.Check("status is 200", func() string {
				if resp.StatusCode != http.StatusOK {
					return fmt.Sprintf("unexpected status: %d", resp.StatusCode)
				}
				return ""
			})

			// Decode JSON response directly
			var result CheckoutResult
			if err := resp.JSON(&result); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}

			return nil
		},
	})

	suite.Execute()
}
```

### Real-Time Server-Sent Events (SSE) Streaming

```go
RunVU: func(ctx vuhive.VUContext) error {
    client := vuhivehttp.Default(ctx)

    // Open SSE stream (Accept: text/event-stream added automatically)
    stream, err := client.StreamSSE(ctx, "/api/v1/chat/completions/stream")
    if err != nil {
        return err
    }
    defer stream.Close()

    var tokens int
    for stream.Next() {
        event := stream.Event()
        if event.Event == "token" {
            tokens++
        }
        if event.Data == "[DONE]" {
            break
        }
    }

    ctx.Check("received_tokens", tokens > 0)
    return stream.Err()
}
```

### Third-Party SDK Integration (`StandardClient` / `Transport`)

When integrating third-party client SDKs (e.g. OpenAI, Anthropic, or custom AI clients) that accept a standard Go `*http.Client` or `http.RoundTripper`, use `client.StandardClient()` or `client.Transport()`:

```go
RunVU: func(ctx vuhive.VUContext) error {
    vClient := vuhivehttp.Default(ctx)

    // Obtain standard *http.Client backed by vuhive instrumentation
    stdClient := vClient.StandardClient()

    // Pass directly to third-party SDK
    sdk := myclient.New(myclient.WithHTTPClient(stdClient))
    return sdk.ChatStream(ctx, "Hello!")
}
```

- **Seamless SSE & REST Routing**: Automatically detects `Accept: text/event-stream` and routes via `DoStream`, piping SSE events into streaming `*http.Response` bodies while routing REST calls via `Do`.
- **Automatic Telemetry**: Records all standard `vuhive.http.*` and `vuhive.http.sse.*` metrics seamlessly.
- **Resource Management**: Closing the returned `resp.Body` immediately tears down the underlying SSE stream.

### Wrapping Pre-Configured Clients (`vuhivehttp.Instrument`)

When bringing your own pre-configured `*http.Client` or `http.RoundTripper` (e.g. AWS SDK, Google Cloud Client, Stripe SDK, or specialized OAuth2 clients), use `vuhivehttp.Instrument()` or `vuhivehttp.InstrumentTransport()`:

```go
Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
    baseClient := &http.Client{Timeout: 5 * time.Second}
    client := vuhivehttp.Instrument(
        baseClient,
        vuhivehttp.WithMetricPrefix("vuhive.http."),
        vuhivehttp.WithTags(vuhive.Tags{"env": "staging"}),
    )
    return map[string]any{"client": client}, nil
},
RunVU: func(ctx vuhive.VUContext) error {
    client := ctx.GlobalState("client").(*http.Client)
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/items", nil)
    resp, err := client.Do(req) // Telemetry is recorded dynamically from ctx
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    return nil
}
```


### Auto-Recorded HTTP & SSE Metrics

| Metric Identifier | Type | Tags | Description |
|---|---|---|---|
| `vuhive.http.req_duration` | Duration (HDR) | `method`, `url`, `status` | Total request latency histogram |
| `vuhive.http.reqs` | Counter | `method`, `url`, `status` | Total HTTP requests count |
| `vuhive.http.req_failed` | Rate | `method`, `url`, `status` | Ratio of failed requests (non-2xx or transport error) |
| `vuhive.http.sse.connections_total` | Counter | `method`, `url`, `status` | Total SSE stream connection attempts |
| `vuhive.http.sse.connect_duration` | Duration (HDR) | `method`, `url`, `status` | Latency to establish SSE connection and receive headers |
| `vuhive.http.sse.events_total` | Counter | `method`, `url`, `event_type` | Total decoded SSE events received |
| `vuhive.http.sse.event_latency` | Duration (HDR) | `method`, `url`, `event_type` | Inter-arrival latency between successive events (TTFE) |
| `vuhive.http.sse.stream_duration` | Duration (HDR) | `method`, `url`, `status` | Total active lifespan of streaming sessions |
| `vuhive.http.sse.errors_total` | Counter | `method`, `url` | SSE stream errors, disconnections, or framing failures |
| `vuhive.http.req_connecting` | Duration (HDR) | `method`, `url`, `status` | TCP connection latency *(opt-in via `WithDetailedTiming`)* |
| `vuhive.http.req_tls_handshaking` | Duration (HDR) | `method`, `url`, `status` | TLS handshake latency *(opt-in via `WithDetailedTiming`)* |
| `vuhive.http.req_sending` | Duration (HDR) | `method`, `url`, `status` | Request payload write latency *(opt-in via `WithDetailedTiming`)* |
| `vuhive.http.req_receiving` | Duration (HDR) | `method`, `url`, `status` | Response body read latency *(opt-in via `WithDetailedTiming`)* |

---

## Kafka Messaging Module (`pkg/vuhive/kafka`)

Test event-driven architectures with high-throughput Kafka Publisher and Consumer clients. To avoid unnecessary dependency trees for non-Kafka workloads, the driver is conditionally compiled using Go build tags.

### Conditional Compilation Architecture

- **Default Builds (`go build .`)**: Compiles a zero-dependency no-op fallback. Operations return `ErrKafkaDisabled` with zero third-party Kafka drivers linked into your binary.
- **Kafka Builds (`go build -tags kafka .`)**: Compiles the high-throughput, pure-Go Kafka driver (`franz-go`) with automatic telemetry.

### Basic Usage (`-tags kafka`)

```go
package main

import (
	"fmt"
	"time"

	"github.com/morphy76/vuhive/pkg/vuhive"
	"github.com/morphy76/vuhive/pkg/vuhive/kafka"
)

func main() {
	suite := vuhive.NewSuite("Kafka Event Stream Load Test")

	suite.RegisterScenario("order_events", vuhive.Scenario{
		Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
			// Initialize shared Kafka Client (Publisher + Consumer)
			client, err := kafka.NewClient(ctx,
				kafka.WithBrokers("localhost:9092"),
				kafka.WithTopic("order_events"),
				kafka.WithGroupID("order_processors"),
				kafka.WithTimeout(5*time.Second),
			)
			if err != nil {
				return nil, err
			}
			return map[string]any{"kafka": client}, nil
		},

		RunVU: func(ctx vuhive.VUContext) error {
			client := ctx.GlobalState("kafka").(kafka.Client)

			// 1. Publish event — latency, message count, bytes, and error rates are auto-recorded
			msg := &kafka.Message{
				Topic: "order_events",
				Key:   []byte(fmt.Sprintf("user-%d", ctx.VUID())),
				Value: []byte(`{"event":"order_created","amount":49.90}`),
				Headers: map[string][]byte{"source": []byte("load-generator")},
			}
			if err := client.Publish(ctx, msg); err != nil {
				return err
			}

			// 2. Consume from stream
			recvMsg, err := client.Consume(ctx)
			if err != nil {
				return err
			}

			if recvMsg != nil {
				// 3. Commit consumer offset
				_ = client.Commit(ctx, recvMsg)
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

### Auto-Recorded Kafka Metrics

| Metric Identifier | Type | Tags | Description |
|---|---|---|---|
| `vuhive.kafka.pub_duration` | Duration (HDR) | `topic`, `status` | Publish round-trip latency histogram |
| `vuhive.kafka.pub_total` | Counter | `topic`, `status` | Total messages published |
| `vuhive.kafka.pub_bytes` | Counter | `topic` | Total payload bytes published |
| `vuhive.kafka.pub_failed` | Rate | `topic`, `status` | Ratio of failed publish operations |
| `vuhive.kafka.sub_duration` | Duration (HDR) | `topic`, `group`, `status` | Message fetch/wait duration |
| `vuhive.kafka.sub_total` | Counter | `topic`, `group`, `status` | Total messages consumed |
| `vuhive.kafka.sub_bytes` | Counter | `topic` | Total payload bytes consumed |
| `vuhive.kafka.sub_failed` | Rate | `topic`, `group`, `status` | Ratio of failed consume operations |

---

## Writing a Load Test (Code Example)

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/morphy76/vuhive/pkg/vuhive"
)

func main() {
	suite := vuhive.NewSuite("E-Commerce Load Test Suite")

	suite.RegisterScenario("http_checkout_flow", vuhive.Scenario{
		Setup: func(ctx vuhive.SetupContext) (map[string]any, error) {
			client := &http.Client{Timeout: 5 * time.Second}
			return map[string]any{"client": client}, nil
		},
		PreTest: func(ctx vuhive.VUContext) error {
			ctx.Log().Debug().Msg("preparing iteration")
			return nil
		},
		RunVU: func(ctx vuhive.VUContext) error {
			baseURL := ctx.Param("base_url")
			client := ctx.GlobalState("client").(*http.Client)

			start := time.Now()
			req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/health", nil)
			resp, err := client.Do(req)
			elapsed := time.Since(start)

			ctx.Metrics().Duration("http_request_duration", vuhive.Tags{}).Observe(elapsed)

			if err != nil || resp.StatusCode != http.StatusOK {
				ctx.Metrics().Rate("checkout_success_rate", vuhive.Tags{}).Add(0, 1)
				return fmt.Errorf("request failed: %v", err)
			}
			_ = resp.Body.Close()

			ctx.Metrics().Rate("checkout_success_rate", vuhive.Tags{}).Add(1, 1)
			ctx.Metrics().Counter("http_requests_total", vuhive.Tags{}).Inc()
			return nil

		},
		AfterTest: func(ctx vuhive.VUContext) error {
			ctx.Log().Debug().Msg("iteration finished")
			return nil
		},
		Teardown: func(ctx vuhive.TeardownContext, state map[string]any) error {
			return nil
		},
		HandleSummary: func(ctx vuhive.SummaryContext, summary vuhive.SummaryData) error {
			fmt.Printf("Summary Hook: %s completed in %v, passed=%v\n", summary.Scenario, summary.Duration, summary.Passed)
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

---

## Execution Summary Hook (`HandleSummary`)

`HandleSummary` enables developers to receive the complete execution summary programmatically after the test run and terminal/JSON report generation. This is ideal for posting results to Slack, Datadog, webhooks, or generating custom artifacts.

```go
HandleSummary: func(ctx vuhive.SummaryContext, summary vuhive.SummaryData) error {
    // Check SLA verdict
    if !summary.Passed {
        sendSlackAlert(fmt.Sprintf("SLA breached for %s!", summary.Scenario))
    }

    // Access metrics & thresholds
    reqCount := summary.Counter("http_requests_total")
    latencyMetric := summary.Metric("http_request_duration")
    fmt.Printf("Processed %d requests, p95 latency: %v\n", reqCount, latencyMetric.P95)

    // Export JSON summary directly
    jsonBytes, _ := summary.JSON()
    os.WriteFile("summary-custom.json", jsonBytes, 0644)
    return nil
}
```

> **Note:** Any error returned by `HandleSummary` is logged to the output stream but does not modify the final exit code.

---


## CLI Options & Execution

Run your test binary with command-line flags:

```bash
go run main.go [flags]
```

### Available CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `vuhive.yaml` | Path to the YAML configuration file. |
| `--scenario` | (default in config) | Specific scenario name to execute. |
| `--log-level` | `info` | Log verbosity: `debug`, `info`, `warn`, `error`. |
| `--log-format` | `pretty` | Log output format: `pretty` or `json`. |
| `--report-format` | `console` | Report format: `console` or `json`. |
| `--report-out` | stdout | File path to write the primary summary report. |
| `--json-report-out` | (none) | File path to write the JSON report document (§10.2 schema). |
| `--version` | `false` | Prints library version info and returns `0`. |

### Exit Code Contract

- **Exit `0`**: Scenario completed and **all SLA thresholds passed**.
- **Exit `1`**: One or more SLA thresholds **failed**, or a pre-execution error occurred.

---

## Output Report Sample

```text
================================================================================
                        VUHIVE LOAD TEST SUMMARY
================================================================================
Scenario:     http_checkout_flow              Version: 0.1.0
Mode:         constant_vus (10 VUs)           Commit:  dev
Duration:     00:00:40  (ramp-up: 5s | run: 30s | ramp-down: 5s)
Iterations:   1,450 total  |  0 failed (0.00%)  |  0 timeout

BUILT-IN METRICS
────────────────────────────────────────────────────────────────
vuhive.vu.iterations_total      Counter    1450
vuhive.vu.iterations_failed     Counter    0
vuhive.vu.iterations_timeout    Counter    0
vuhive.vu.panics                Counter    0
vuhive.vu.pretest_errors        Counter    0

CUSTOM METRICS
────────────────────────────────────────────────────────────────
Metric                         Type       Count    Min     Mean    p95     p99     Max
checkout_success_rate          Rate       1450     (rate: 1)
http_request_duration          Duration   1450     12ms    45ms    110ms   230ms   850ms
http_requests_total            Counter    1450

SLA THRESHOLD EVALUATION
────────────────────────────────────────────────────────────────
  [PASS]  http_request_duration   p95 < 200ms     → actual: 110ms
  [PASS]  checkout_success_rate   rate >= 0.99    → actual: 1
  [PASS]  http_requests_total     count >= 500    → actual: 1450
────────────────────────────────────────────────────────────────
OVERALL: PASSED                                          (exit 0)
================================================================================
```

---

## Benchmarking & Framework Overhead

The framework is optimized to ensure maximum load generation throughput with zero GC pressure during steady-state execution:

- **0 allocs/op** per Virtual User iteration in steady state (`constant_vus`, `ramping_vus`, `arrival_rate`).
- **100% Lock-Free** metric queries leveraging copy-on-write atomic pointer maps and 16-stripe sharded HDR Histograms.
- **Sub-10ns Inline Checks**: `ctx.Check()` evaluates in ~6.4ns with cached metric handles and zero allocations.

Run the microbenchmark and performance verification suite:

```bash
# Run in-tree microbenchmarks:
make test-bench

# Run deterministic zero-allocation regression checks and hot-path benchmarks:
make test-perf
```

For complete architectural details, performance verification targets, `pprof` profiling guides, Go runtime tuning (`GOMEMLIMIT`, `GOGC`, `GOMAXPROCS`), and multi-engine comparisons, see the [Performance & Verification Suite Guide](docs/BENCHMARKS.md).

---

## Development & Architecture

For contributors, architecture guidelines (Hexagonal/DDD), technology stack constraints, structured logging patterns, and the strict TDD development cycle, see the [**Development Guide**](docs/DEVELOPMENT.md).

---

## Examples & Reference Implementations

The [`examples/`](examples/README.md) directory contains self-contained, compilable load test suites demonstrating all framework capabilities. See the [**Examples Reference Suite**](examples/README.md) for a structured 3-tier learning progression path and complete feature matrix.

| Example Directory | Scenario Type | Features Demonstrated | Documentation |
|---|---|---|---|
| [`examples/http_checkout/`](examples/http_checkout/) | `constant_vus` | REST API load test, manual duration/counter/rate metrics, linear ramp-up/down. | [Guide](examples/http_checkout/README.md) |
| [`examples/http_module/`](examples/http_module/) | `constant_vus` | Built-in `pkg/vuhive/http` client, auto-recorded metrics, JSON decoding, inline checks. | [Guide](examples/http_module/README.md) |
| [`examples/checks/`](examples/checks/) | `constant_vus` | Inline assertions (`ctx.Check`) for HTTP status, headers, JSON body validation, check metrics. | [Guide](examples/checks/README.md) |
| [`examples/groups/`](examples/groups/) | `constant_vus` | Transaction boundaries (`ctx.Group`), nested groups, group-scoped duration metrics, threshold targeting. | [Guide](examples/groups/README.md) |
| [`examples/think_time/`](examples/think_time/) | `constant_vus` | Multi-step user journey, declarative `interaction_delay` (`range`), `ctx.Sleep()`, programmatic `ExpoDelay`. | [Guide](examples/think_time/README.md) |
| [`examples/data_parameterization/`](examples/data_parameterization/) | `constant_vus` | CSV (`Sequential`), JSON (`Random`), and JSONL (`SharedQueue`) dataset parameterization. | [Guide](examples/data_parameterization/README.md) |
| [`examples/ramping_vus/`](examples/ramping_vus/) | `ramping_vus` | Multi-stage spike test with dynamic VU scaling and recovery observation. | [Guide](examples/ramping_vus/README.md) |
| [`examples/sla_thresholds/`](examples/sla_thresholds/) | `constant_vus` | Quality gate SLA thresholds across metrics, percentile operators, and early stop with `abort_on_fail`. | [Guide](examples/sla_thresholds/README.md) |
| [`examples/handle_summary/`](examples/handle_summary/) | `constant_vus` | Post-test execution hook (`HandleSummary`), summary metric inspection, webhook notification dispatch. | [Guide](examples/handle_summary/README.md) |
| [`examples/conversation_flow/`](examples/conversation_flow/) | `constant_vus` | Real-time SSE streaming conversational AI load test, multi-turn state machine, DSL client. | [Guide](examples/conversation_flow/README.md) |
| [`examples/sse_streaming/`](examples/sse_streaming/) | `constant_vus` | Built-in `pkg/vuhive/http` Server-Sent Events (SSE) streaming, TTFE latency, token throughput. | [Guide](examples/sse_streaming/README.md) |
| [`examples/grpc_user_service/`](examples/grpc_user_service/) | `arrival_rate` | High-throughput RPC simulation, token bucket TPS pacing, bounded worker pool. | [Guide](examples/grpc_user_service/README.md) |
| [`examples/kafka/`](examples/kafka/) | `constant_vus` | Event streaming with `pkg/vuhive/kafka` publisher and consumer, build tag `-tags kafka`. | [Guide](examples/kafka/README.md) |

### Running Examples

All examples use in-process mock servers and can be executed immediately:

```bash
# Run any example directly from its folder:
cd examples/think_time && go run -tags=vuhive_example .

# Verify compilation across all example binaries:
make test-examples
```


