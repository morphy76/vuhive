# NATS Messaging & RPC Load Test Example

Demonstrates the **vuhive NATS module** (`pkg/vuhive/nats`), which provides auto-instrumented NATS Publisher, Subscriber, and Request-Reply clients conditionally compiled using Go build tags (`//go:build nats`).

## Concept Overview

Testing event-driven systems and microservice meshes requires generating concurrent message flows, verifying pub/sub topologies, and measuring Request-Reply RPC round-trip latencies under varying load levels.

The `vuhive/nats` module eliminates boilerplate code and provides zero-dependency builds for test suites that do not exercise NATS.

### Conditional Compilation

| Build Command | Active Driver | Behavior |
|---|---|---|
| `go build .` | No-Op (`!nats`) | Zero external NATS dependencies linked in binary. Operations return `ErrNATSDisabled`. |
| `go build -tags nats .` | Concrete Driver (`nats`) | Full-featured NATS driver (`nats.go`) with automatic telemetry. |

## Key Files

| File | Description |
|---|---|
| `main.go` | Scenario using `nats.NewClient(ctx)` for publishing, synchronous subscriptions, and Request-Reply RPC |
| `vuhive.yaml` | Load profile configuration with SLA thresholds on NATS metrics |

## How to Run

```bash
# Run standalone with the in-process mock NATS server:
go run -tags "vuhive_example nats" ./examples/nats

# Or specify custom configuration:
go run -tags "vuhive_example nats" ./examples/nats --config ./examples/nats/vuhive.yaml
```

## Automatic Metrics

The NATS module records the following telemetry metrics:

| Metric | Type | Tags | Description |
|---|---|---|---|
| `vuhive.nats.pub_duration` | Duration | `subject`, `status` | Publish latency histogram |
| `vuhive.nats.pub_total` | Counter | `subject`, `status` | Total messages published |
| `vuhive.nats.pub_bytes` | Counter | `subject` | Total payload bytes published |
| `vuhive.nats.pub_failed` | Rate | `subject`, `status` | Ratio of failed publish operations |
| `vuhive.nats.req_duration` | Duration | `subject`, `status` | Request-Reply round-trip latency histogram |
| `vuhive.nats.sub_received_total` | Counter | `subject`, `status` | Total messages received |
| `vuhive.nats.sub_bytes` | Counter | `subject` | Total payload bytes received |
| `vuhive.nats.sub_failed` | Rate | `subject`, `status` | Ratio of failed receive operations |

## Configuration & Thresholds

```yaml
thresholds:
  - metric: vuhive.nats.pub_duration
    stat: p95
    operator: "<"
    target: "10ms"

  - metric: vuhive.nats.req_duration
    stat: p95
    operator: "<"
    target: "20ms"

  - metric: vuhive.nats.pub_failed
    stat: rate
    operator: "<="
    target: "0.01"
```
