# Inline Assertions (Checks) Example

A focused reference example demonstrating how to validate functional conditions during load test iterations using `ctx.Check()`.

---

## Concept Overview

In load testing, validating API correctness without prematurely aborting virtual users is essential:
- **Convenience Context Assertions**: `ctx.CheckEqual()`, `ctx.CheckTrue()`, and `ctx.CheckNoError()` provide clean, expressive one-line assertions with zero allocations on passing checks.
- **Composable Assertion Generators**: `vuhive.Equal()`, `vuhive.True()`, `vuhive.NoError()`, `vuhive.Contains()`, and `vuhive.InRange()` return standard `CheckFunc` closures for use with `ctx.Check(name, fn)`.
- **Custom Closures**: `ctx.Check(name, fn)` supports custom closures returning `""` (empty string) on pass, or a descriptive reason string on failure.
- **Non-Fatal**: If a check fails, the iteration continues normally, recording the failure for statistical analysis.
- **Auto-Instrumentation**: `vuhive` automatically tracks `vuhive.checks.passed` and `vuhive.checks.failed` counters tagged with the check name.
- **Dedicated Reporting**: Check pass rates and failure counts are displayed in a formatted `CHECKS` summary table.
- **SLA Threshold Integration**: You can define SLA quality gates on check metrics (e.g. `vuhive.checks.failed count <= 0`).

---

## Key Files

| File | Description |
|---|---|
| [`main.go`](main.go) | Scenario demonstrating direct context methods (`CheckEqual`, `CheckNoError`, `CheckTrue`) and assertion generator (`vuhive.Contains`). Includes an in-process mock API server. |
| [`vuhive.yaml`](vuhive.yaml) | Configuration with SLA thresholds asserting zero check failures (`vuhive.checks.failed <= 0`). |

---

## How to Run

From the repository root:

```bash
go run -tags=vuhive_example ./examples/checks --config ./examples/checks/vuhive.yaml
```

Or from within the example directory:

```bash
cd examples/checks
go run -tags=vuhive_example .
```

---

## Configuration Breakdown (`vuhive.yaml`)

```yaml
version: "1.0"
default_scenario: checks_demo

scenarios:
  checks_demo:
    type: constant_vus
    vus: 4
    ramp_up: 50ms
    run_period: 300ms
    ramp_down: 50ms
    vu_timeout: 1s

    thresholds:
      # Enforce zero check failures across all iterations
      - metric: vuhive.checks.failed
        stat: count
        operator: "<="
        target: "0"

      # Ensure at least 10 checks executed and passed
      - metric: vuhive.checks.passed
        stat: count
        operator: ">="
        target: "10"
```

---

## Expected Output

```text
================================================================================
                        VUHIVE LOAD TEST SUMMARY
================================================================================
Scenario:     checks_demo                     Version: dev
Mode:         constant_vus (4 VUs)            Commit:  none
Duration:     00:00:00  (ramp-up: 50ms | run: 300ms | ramp-down: 50ms)
Iterations:   14821 total  |  0 failed (0.00%)  |  0 timeout

BUILT-IN METRICS
────────────────────────────────────────────────────────────────
vuhive.vu.iterations_total      Counter    14821
vuhive.vu.iterations_failed     Counter    0
vuhive.vu.iterations_timeout    Counter    0
vuhive.vu.panics                Counter    0
vuhive.vu.pretest_errors        Counter    0
vuhive.checks.passed            Counter    74105
vuhive.checks.failed            Counter    0

CHECKS
────────────────────────────────────────────────────────────────
Check Name                     Passed     Failed   Pass %  
content-type is json           14821      0        100.00%
response body is valid json    14821      0        100.00%
response message is present    14821      0        100.00%
response status is success     14821      0        100.00%
status code is 200             14821      0        100.00%

SLA THRESHOLD EVALUATION
────────────────────────────────────────────────────────────────
  [PASS]  vuhive.checks.failed     count <= 0      → actual: 0
  [PASS]  vuhive.checks.passed     count >= 10     → actual: 74105
────────────────────────────────────────────────────────────────
OVERALL: PASSED                                         (exit 0)
================================================================================
```
