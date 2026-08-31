# HTTP Checkout Flow Example

A foundational reference example demonstrating standard REST API load testing with `vuhive` using standard `*http.Client` wrapped with `vuhivehttp.Instrument()`.

---

## Concept Overview

This example demonstrates the fundamental workflow of a `vuhive` load test:
- Initializing shared, thread-safe resources (like an `*http.Client`) once during the `Setup` lifecycle phase.
- Wrapping the standard client with `vuhivehttp.Instrument(client)` for automatic latency, counter, and failure rate metric recording.
- Passing shared handles to Virtual Users (VUs) via `GlobalState`.
- Reading scenario configuration parameters dynamically via `ctx.Param()`.
- Executing standard `client.Do(req)` in `RunVU` without repetitive manual telemetry code.
- Defining and evaluating declarative SLA threshold quality gates in `vuhive.yaml`.

---

## Key Files

| File | Description |
|---|---|
| [`main.go`](main.go) | Scenario definition, lifecycle hooks (`Setup`, `PreTest`, `RunVU`, `AfterTest`, `Teardown`), `vuhivehttp.Instrument` client wrapping, and HTTP request execution. Includes an in-process mock HTTP backend server. |
| [`vuhive.yaml`](vuhive.yaml) | Declarative load profile configuration specifying `constant_vus` concurrency, durations, custom parameters, and SLA quality gates. |

---

## How to Run

Run the example directly from the repository root:

```bash
go run -tags=vuhive_example ./examples/http_checkout --config ./examples/http_checkout/vuhive.yaml
```

Or from within the example directory:

```bash
cd examples/http_checkout
go run -tags=vuhive_example .
```

---

## Configuration Breakdown (`vuhive.yaml`)

```yaml
version: "1.0"
default_scenario: http_checkout_flow

scenarios:
  http_checkout_flow:
    type: constant_vus   # Closed-system model with fixed concurrent VUs
    vus: 5               # 5 concurrent Virtual Users executing in parallel
    ramp_up: 100ms       # Linear staggered spawn over 100ms
    run_period: 500ms    # Steady-state execution duration
    ramp_down: 100ms     # Grace period for in-flight requests
    vu_timeout: 1s       # Per-iteration context deadline

    params:
      checkout_path: "/checkout"  # Accessed via ctx.Param("checkout_path")

    thresholds:
      - metric: vuhive.http.req_duration
        stat: p95
        operator: "<"
        target: "200ms"           # 95th percentile latency must be under 200ms
      - metric: vuhive.http.req_failed
        stat: rate
        operator: "<="
        target: "0.05"            # Failure rate must be <= 5%
```

---

## Expected Output

```text
================================================================================
                        VUHIVE LOAD TEST SUMMARY
================================================================================
Scenario:     http_checkout_flow              Version: dev
Mode:         constant_vus (5 VUs)            Commit:  none
Duration:     00:00:00  (ramp-up: 100ms | run: 500ms | ramp-down: 100ms)
Iterations:   389 total  |  0 failed (0.00%)  |  0 timeout

BUILT-IN METRICS
────────────────────────────────────────────────────────────────
vuhive.vu.iterations_total      Counter    389
vuhive.vu.iterations_failed     Counter    0
vuhive.vu.iterations_timeout    Counter    0
vuhive.vu.panics                Counter    0
vuhive.vu.pretest_errors        Counter    0
vuhive.checks.passed            Counter    0
vuhive.checks.failed            Counter    0

HTTP METRICS
────────────────────────────────────────────────────────────────
Metric                         Type       Count    Min     Mean    p95     p99     Max    
vuhive.http.req_duration       Duration   389      5.628ms 7.217ms 8.519ms 9.367ms 11.495ms
vuhive.http.req_failed         Rate       (rate: 0)
vuhive.http.reqs               Counter    389     

SLA THRESHOLD EVALUATION
────────────────────────────────────────────────────────────────
  [PASS]  vuhive.http.req_duration   p95 < 200ms     → actual: 8.519ms
  [PASS]  vuhive.http.req_failed     rate <= 0.05    → actual: 0
────────────────────────────────────────────────────────────────
OVERALL: PASSED                                         (exit 0)
================================================================================
```
