# gRPC / RPC Service Load Test Example

A reference example demonstrating protocol extensibility and open-system rate pacing (`arrival_rate`) with bounded worker pools.

---

## Concept Overview

While HTTP/REST is common, `vuhive` is protocol-agnostic and well-suited for high-throughput RPC load testing:
- **Protocol Agnostic**: The `RunVU` function executes pure Go code—allowing load testing of gRPC (`google.golang.org/grpc`), tRPC, Twirp, database queries, or raw TCP/UDP protocols.
- **Open-System Pacing (`arrival_rate`)**: Targets a precise Transactions Per Second (TPS) arrival rate driven by a token bucket (`golang.org/x/time/rate`).
- **Bounded Worker Pool (`max_vus`)**: Pre-allocates up to `max_vus` persistent worker goroutines to process arriving tokens, eliminating per-iteration goroutine creation overhead.
- **Tagged Latencies**: Recording HDR duration histograms tagged with `service` and `method` dimensions.
- **Connection Lifecycle**: Initializing connection pools (`grpc.ClientConn`) once in `Setup` and closing them cleanly in `Teardown`.

---

## Key Files

| File | Description |
|---|---|
| [`main.go`](main.go) | Scenario demonstrating RPC execution, open-system arrival rate handling, and tagged metrics. Includes an in-memory mock user store. |
| [`vuhive.yaml`](vuhive.yaml) | Configuration with `arrival_rate` pacing (`target_tps: 20`, `max_vus: 20`) and latency thresholds. |

---

## How to Run

From the repository root:

```bash
go run -tags=vuhive_example ./examples/grpc_user_service --config ./examples/grpc_user_service/vuhive.yaml
```

Or from within the example directory:

```bash
cd examples/grpc_user_service
go run -tags=vuhive_example .
```

---

## Configuration Breakdown (`vuhive.yaml`)

```yaml
version: "1.0"
default_scenario: grpc_user_service_flow

scenarios:
  grpc_user_service_flow:
    type: arrival_rate   # Open-system rate-limiting model
    target_tps: 20       # Target rate: 20 iterations/second
    max_vus: 20          # Maximum worker pool concurrency cap
    ramp_up: 100ms       # Linear TPS rate ramp-up
    run_period: 500ms    # Steady-state duration
    ramp_down: 100ms     # Grace period for in-flight requests
    vu_timeout: 1s       # Per-iteration timeout

    params:
      service_name: "UserService"
      method: "GetUser"

    thresholds:
      - metric: grpc_latency
        stat: p95
        operator: "<"
        target: "100ms"
      - metric: rpc_success_rate
        stat: rate
        operator: ">="
        target: "0.95"
```

---

## Expected Output

```text
================================================================================
                        VUHIVE LOAD TEST SUMMARY
================================================================================
Scenario:     grpc_user_service_flow          Version: dev
Mode:         arrival_rate (20 TPS, max 20 VUs)  Commit:  none
Duration:     00:00:00  (ramp-up: 100ms | run: 500ms | ramp-down: 100ms)
Iterations:   11 total  |  0 failed (0.00%)  |  0 timeout

BUILT-IN METRICS
────────────────────────────────────────────────────────────────
vuhive.vu.iterations_total      Counter    11
vuhive.vu.iterations_failed     Counter    0
vuhive.vu.iterations_timeout    Counter    0
vuhive.vu.panics                Counter    0
vuhive.vu.pretest_errors        Counter    0
vuhive.checks.passed            Counter    0
vuhive.checks.failed            Counter    0

CUSTOM METRICS
────────────────────────────────────────────────────────────────
Metric                         Type       Count    Min     Mean    p95     p99     Max    
grpc_calls_total               Counter    11      
grpc_latency                   Duration   11       2.286ms 4.649ms 6.795ms 6.807ms 6.807ms
rpc_success_rate               Rate       (rate: 1)

SLA THRESHOLD EVALUATION
────────────────────────────────────────────────────────────────
  [PASS]  grpc_latency            p95 < 100ms     → actual: 6.795ms
  [PASS]  rpc_success_rate        rate >= 0.95    → actual: 1
────────────────────────────────────────────────────────────────
OVERALL: PASSED                                         (exit 0)
================================================================================
```
