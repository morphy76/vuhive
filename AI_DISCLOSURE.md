# AI Disclosure & Spec-Driven Development (SDD)

## Overview & Philosophy

`vuhive` is an open-source, high-performance Go load testing framework engineered from the ground up following **Spec-Driven Development (SDD)**. It actively embraces and fosters the collaborative adoption of **autonomous coding agents** (such as Google Antigravity, Anthropic Claude Code, GitHub Copilot Workspace, and related AI pair-programming systems).

In modern software engineering, AI assistance is often treated as an ad-hoc autocomplete or a source of opaque code generation. `vuhive` rejects that unstructured approach. Instead, we treat artificial intelligence as a first-class participant in the development lifecycle governed by formal specifications, rigorous architectural boundaries, and deterministic automated testing.

---

## The Core Paradigm Shift: "What" vs. "How"

The central tenet of engineering `vuhive` is the explicit separation of cognitive responsibilities between human engineers and autonomous coding agents:

```text
 ┌─────────────────────────────────────────────────────────────┐
 │                       HUMAN ENGINEER                        │
 │                                                             │
 │   • Strategic Intent & Problem Definition ("What")          │
 │   • Domain Modeling & System Boundaries                     │
 │   • Architectural Blueprint & Invariant Definition          │
 │   • Performance Budgets & Threat Modeling                   │
 │   • Governance, Ethical Oversight & Final Review            │
 └──────────────────────────────┬──────────────────────────────┘
                                │ Formal Specifications
                                │ (SPECIFICATION.md, Rules, ADRs)
                                ▼
 ┌─────────────────────────────────────────────────────────────┐
 │                   AUTONOMOUS CODING AGENT                   │
 │                                                             │
 │   • Implementation Synthesis ("How")                        │
 │   • Strict TDD Test Suite Generation (Red-Green-Refactor)   │
 │   • Static Compile-Time Interface Assertions                │
 │   • Zero-Allocation Hot-Path Optimization                   │
 │   • Documentation, Examples & Schema Synchronization        │
 └──────────────────────────────┬──────────────────────────────┘
                                │ Deterministic Artifacts
                                │ (Code, Tests, Docs, Benchmarks)
                                ▼
 ┌─────────────────────────────────────────────────────────────┐
 │                AUTOMATED QUALITY GATES & CI                 │
 │                                                             │
 │   • Go 1.26 Compiler Verification                           │
 │   • Thread Sanitizer / Race Detector (-race)                │
 │   • Zero-Allocation Regression Microbenchmarks              │
 │   • Strict Static Analysis (golangci-lint)                  │
 └─────────────────────────────────────────────────────────────┘
```

### 1. The Human Role: Defining the "What"
Human software engineers focus exclusively on high-level cognitive and architectural decisions:
- **Requirements & Domain Invariants**: Formulating what problem must be solved, why it matters, and what invariants must never be violated.
- **Architectural Design**: Designing domain aggregates, ports, adapters, dependency directions, and lifecycle stages.
- **Constraints & Budgets**: Specifying memory boundaries, zero-allocation targets on the hot path, and acceptable latency percentiles.
- **Verification & Review**: Serving as the final authority on pull request reviews, evaluating design coherence, and confirming that the synthesized solution honors the system architecture.

### 2. The Agent Role: Executing the "How"
Autonomous coding agents operate within clearly defined sandbox boundaries to handle implementation mechanics:
- **Translating Specifications**: Transforming written requirements and architectural models into clean, idiomatic Go.
- **TDD Test Authoring**: Writing unit, integration, and benchmark tests *before* production code (Red-Green-Refactor).
- **Interface Verification**: Embedding compile-time static type assertions (`var _ Interface = (*Concrete)(nil)`).
- **Boilerplate & Plumbing**: Generating repetitive plumbing (DTO mappings, constructors, options) without introducing human fatigue-induced defects.
- **Mechanical Refactoring**: Restructuring code for readability, performance, and adherence to clean code rules.

---

## Spec-Driven Development (SDD) Framework

In an agentic development model, natural-language prompts alone are insufficient—without structured context, agents will inevitably introduce subtle behavioral and architectural drift.

`vuhive` solves this by anchoring every development activity to formal specifications acting as the single source of truth:

1. **System Specification ([`SPECIFICATION.md`](SPECIFICATION.md))**:
   The comprehensive technical blueprint detailing module layouts, public APIs, execution engines, pacing models, metric registries, memory layout, and configuration schemas.
2. **Behavioral Agent Rules ([`.agents/rules/`](.agents/rules/))**:
   Actionable, machine-readable engineering guardrails that all agents must ingest:
   - **`code-architecture.md`**: Enforces Hexagonal Architecture, pure Domain-Driven Design (DDD) layers, and reactive Go concurrency patterns.
   - **`golang.md`**: Enforces idiomatic Go conventions and mandatory compile-time static interface verification.
   - **`nonfunctional.md`**: Locks the approved technology stack (Go 1.26, Gin, Zerolog, Testify, Viper), SemVer release mechanics, structured logging formats, and clean SOLID principles.
   - **`tdd.md`**: Mandates strict Red-Green-Refactor Test-Driven Development; no production code may be written without a preceding failing test.
3. **Operational Guides & Quality Gates**:
   - [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md): Contribution workflows, release automation, and coding standards.
   - [`docs/BENCHMARKS.md`](docs/BENCHMARKS.md): Performance budgets, steady-state zero-allocation requirements, and profiling procedures.
   - [`schemas/vuhive.schema.json`](schemas/vuhive.schema.json): Strict JSON schema governing scenario configurations.

---

## Elevated Engineering Standards for AI-Assisted Code

A common misconception is that leveraging AI coding agents lowers software craftsmanship standards. In `vuhive`, adopting coding agents **raises** the required standards of rigor. Because coding agents can write code orders of magnitude faster than humans, automated verification and architectural guardrails must be significantly stricter to prevent technical debt accumulation:

| Standard | Traditional Human Development | `vuhive` Agentic Development |
| :--- | :--- | :--- |
| **Interface Satisfaction** | Implicit satisfaction; compiler checks at call sites only. | **Mandatory static compile-time assertions** (`var _ Interface = (*Concrete)(nil)`) on all adapter structs. |
| **Testing Discipline** | Often written post-hoc; varying coverage. | **Strict TDD Red-Green-Refactor cycle** required. Every feature starts with a failing test validating public contracts. |
| **Layer Boundaries** | Prone to convenience shortcuts (e.g. importing adapters into domain). | **Strict Hexagonal / DDD isolation**. Zero third-party or serialization tags in domain models. |
| **Logging Quality** | Inconsistent `fmt.Printf` or unstructured strings. | **Context-aware Zerolog** with structured fields and mandatory enter/exit telemetry (`Debug` on start, `Info`/`Error` on exit with duration). |
| **Performance Verification** | Spot-checked periodically or ignored until scale limits hit. | **Deterministic zero-allocation regression suites** (`make test-perf`) ensuring 0 allocs/op in steady-state VU loops. |
| **Static Analysis** | Optional or relaxed linting. | **Zero-warning policy** with `golangci-lint` and race detection (`go test -race`). |

---

## Guidelines for Contributors & Autonomous Agents

Whether you are a human engineer, an AI coding agent, or a human collaborating with an agent, all contributions to `vuhive` must follow this workflow:

1. **Spec First, Code Second**:
   Do not modify code without first establishing the specification. When proposing a new feature or architectural change, update or draft the specification (`SPECIFICATION.md` or an RFC GitHub issue) and agree on the "What".
2. **Adhere to Agent Rules**:
   Ensure your coding assistant has ingested the rules located in `.agents/rules/`.
3. **Execute the Red-Green-Refactor Cycle**:
   - Write failing unit/integration tests establishing the API contract.
   - Implement minimal passing code.
   - Refactor for performance, memory footprint, and architectural cleanliness.
4. **Run Verification Targets**:
   ```bash
   make lint                  # Run static analysis
   make test                  # Run unit tests
   make test-race             # Verify thread safety and race freedom
   make test-perf             # Verify zero-allocation budgets
   make test-examples         # Verify all reference implementations compile
   ```
5. **Human Accountability & Transparent Attribution**:
   Every pull request merged into `vuhive` is reviewed and approved by human maintainers. All automated agent activity is openly documented in pull requests and commit histories.

---

## Summary

`vuhive` proves that autonomous coding agents and high-performance systems engineering are not mutually exclusive—they are mutually reinforcing. When humans provide clear specifications and rigorous architectural boundaries ("What"), coding agents deliver unprecedented speed, coverage, and implementation consistency ("How").
