# vuhive Framework Developer Guidelines

This document defines the development standards, release workflow, and architecture rules for developers and autonomous agents contributing to the `vuhive` framework. For our core engineering philosophy, Spec-Driven Development (SDD) model, and the division of responsibility between human engineers ("what") and coding agents ("how"), see [**AI_DISCLOSURE.md**](../AI_DISCLOSURE.md).

---

## 1. Release Process

`vuhive` follows the **Quarkus release pattern**: releases are fully automated, pull-request driven, and require no manual workflow dispatching or configuration parameterization.

```text
 ┌──────────────────────┐        ┌───────────────────────┐        ┌──────────────────────┐
 │ Developer bumps      │        │ PR CI Checks Pass     │        │ Release Workflow     │
 │ VERSION.vuhive in PR  ├───────►│ & PR Merged to main   ├───────►│ Runs Tests, Tags     │
 │ (Atomic Commit)      │        │                       │        │ & Publishes Release  │
 └──────────────────────┘        └───────────────────────┘        └──────────────────────┘
```

### Release Workflow Mechanics

1. **Semantic Version Tracking**:
   - The framework version is maintained in the root file `VERSION.vuhive` using Semantic Versioning 2.0.0 (`MAJOR.MINOR.PATCH`).

2. **Atomic Version Bump Pull Request**:
   - Releases are initiated by opening a standalone pull request that modifies only `VERSION.vuhive` (e.g., bumping `0.1.2` to `0.1.3`).
   - Per project non-functional requirements, version bumps must be isolated atomic commits (never mixed with functional code changes).

3. **PR Validation & Review**:
   - All standard CI workflows (linting, unit tests, race detector, example builds) run against the PR.
   - The PR is reviewed and merged into `main` following repository branch protection rules.

4. **Automated Release on Merge**:
   - Merging a PR that updates `VERSION.vuhive` triggers `.github/workflows/release.yml` on `push` to `main`:
     ```yaml
     on:
       push:
         branches:
           - main
         paths:
           - 'VERSION.vuhive'
     ```
   - The workflow automatically executes the following steps:
     1. Runs the complete test suite (`make test`, `make test-race`, `make test-examples`).
     2. Reads the new version string from `VERSION.vuhive`.
     3. Tags the merge commit with `v${VERSION}` (`git tag -a "v${VERSION}" -m "Release v${VERSION}"`).
     4. Pushes the tag to GitHub (`git push origin "v${VERSION}"`).
     5. Creates the official GitHub Release with auto-generated release notes via `gh release create`.

---

## 2. Agent Rules to Honor

All development must strictly adhere to the following architecture, non-functional, and testing rules.

### A. Code Architecture Guidelines

#### 1. Hexagonal Architecture with DDD Boundaries
- **Layering & Import Rules**:
  - **Domain Layer (`domain/` or `pkg/vuhive` domain models)**: Pure business logic, aggregates, entities, value objects, domain errors, and stateless domain services. Must **never** import from `application/` or `adapters/` layers.
  - **Application Layer (`application/`)**: Use case orchestration and workflow coordination. Defines driving interfaces (`ports/inbound/`) and driven interfaces (`ports/outbound/`). Must **never** import from `adapters/` layer.
  - **Adapters Layer (`adapters/`)**: Infrastructure bindings (HTTP handlers, storage adapters, metrics collectors). Driving adapters (`adapters/inbound/`) and driven adapters (`adapters/outbound/`) implement the port interfaces defined by the application layer.
- **Dependency Injection**:
  - High-level services depend exclusively on port interfaces (`application/ports/`), never on concrete adapter structs.
- **Data Mapping & DTO Boundaries**:
  - Inbound DTOs (e.g. HTTP JSON request/response payloads) reside strictly in `adapters/inbound/`.
  - Outbound DTOs (e.g. persistence/database models) reside strictly in `adapters/outbound/`.
  - Domain models remain pure; never pass framework serialization tags (e.g., `json:"..."`, `mapstructure:"..."`, `db:"..."`, `binding:"..."`) into domain or application layers.
  - Adapters are responsible for mapping between external DTOs and domain models.
- **Error Handling & Layer Translation**:
  - Domain errors (e.g., `ErrNotFound`, `ErrInvalidState`, `ErrConflict`) are defined in domain models.
  - Outbound adapters map infrastructure/driver errors into domain errors before returning them to application services.
  - Inbound adapters map domain errors into protocol-specific responses (e.g. HTTP status codes). Never leak raw driver errors to callers.
- **Pragmatism vs. Over-Engineering**:
  - Read paths/queries can use lightweight read models without heavy aggregate reconstruction.
  - Avoid bloated aggregates for simple single-entity CRUD operations; enforce strict DDD aggregates where rich invariants or complex state transitions exist.

#### 2. Reactive Programming & Concurrency Standards
- **Concurrency Primitives**: Use Go Goroutines, Channels, and `select` blocks for responsive, non-blocking pipelines.
- **Context Propagation**: Standard `context.Context` must be actively propagated across all asynchronous boundaries, handling cancellation and timeouts.
- **Resource Limits & Worker Pools**: Never spawn unbounded goroutines (`go func()`). Use bounded worker pools, buffered channels, or semaphores (`errgroup.Group`) to throttle concurrency.
- **Graceful Teardown**: Concurrent background routines must monitor `ctx.Done()` and coordinate termination via `sync.WaitGroup` or `errgroup.Group` to prevent dropped tasks or leaks.

---

### B. General Non-Functional Requirements & System Constraints

#### 1. Approved Technology Stack
The approved technology stack is strictly locked:
- **Core Language:** Go 1.26
- **HTTP Routing:** Gin (`github.com/gin-gonic/gin`)
- **Structured Logging:** Zerolog (`github.com/rs/zerolog`)
- **Testing Libraries:** Testify (`github.com/stretchr/testify`) and Testcontainers-Go
- **Configuration Engine:** Viper (`github.com/spf13/viper`)

#### 2. Monorepo Build System (`Makefile`)
- All Makefile targets must be explicitly declared `.PHONY`.
- Versioning is injected at compile time via `ldflags` reading `VERSION.vuhive` and Git metadata into package variables in `internal/version/`.
- Required targets: `test`, `test-integration`, `test-bench`, `test-perf`, `verify-performance`, `test-race`, `test-examples`, `lint`, `generate`, and `help`.
- For the performance verification matrix, zero-allocation budgets, and profiling procedures, refer to [BENCHMARKS.md](BENCHMARKS.md).

#### 3. Structured Logging Strategy (`zerolog`)
- Use `github.com/rs/zerolog` for JSON structured logging.
- Extract contextual loggers using `zerolog.Ctx(ctx)`.
- Use structured typed fields; never format log messages with `fmt.Sprintf`.
- **Public Function Enter/Exit Logging Pattern**:
  - **On Enter (Debug)**: Emit a `Debug` event logging input parameters ("starting <operation>").
  - **On Exit (Info/Error)**: Emit an `Info` event ("completed <operation>") including execution duration (`time.Since(start)`), or an `Error` event with error context on failure.
- **Security**: Never log sensitive tokens, credentials, or PII.

#### 4. Graceful Teardown & OS Signals
- Listen for `os.Interrupt`, `syscall.SIGINT`, and `syscall.SIGTERM`.
- Provide configurable shutdown timeout contexts to flush in-flight operations.

#### 5. Clean Code & SOLID Principles
- **Single Responsibility (SRP)**: Each package, struct, and function must have one focused responsibility. Avoid God structs/functions.
- **Open/Closed (OCP)**: Favor interface composition, functional options, or strategy patterns over hardcoded type-switch statements.
- **Liskov Substitution (LSP)**: Interface implementations must honor full behavioral contracts without unexpected panics or side-effects.
- **Interface Segregation (ISP)**: Prefer small, client-defined interfaces (1–3 methods).
- **Dependency Inversion (DIP)**: Depend on abstractions (port interfaces), never concrete implementations.
- **Left-Aligned Happy Path**: Use guard clauses and early returns for errors/edge cases to keep success logic left-aligned.
- **Explicit Error Handling**: Never ignore errors with `_`; explicitly wrap or translate errors with domain context.
- **Avoid Global State**: Do not rely on package-level mutable variables or implicit `init()` side-effects.

---

### C. Strict Test-Driven Development (TDD) Discipline

All features, refactors, and bug fixes must follow the **Red-Green-Refactor** cycle:

#### Phase 1: Red (Design API & Write Failing Test First)
- Write a concise, focused test specifying intended unit behavior and consumer interaction before writing implementation code.
- Execute the test and verify it fails for the expected reason (missing symbol, assertion failure).

#### Phase 2: Green (Fastest Path to Pass)
- Write the simplest, most direct implementation required to make the failing test pass.
- Do not attempt premature optimization or structural refactoring during this phase.

#### Phase 3: Refactor (Optimization, Maintainability, & Architecture)
- With all tests passing (green), clean up code duplication, improve naming, optimize memory/performance, and align with SOLID/Hexagonal architecture.
- Continuously re-run the full test suite to guarantee zero regression.

#### Strict Execution Guidelines
- **No Code Without Tests**: Production logic must never be introduced without a driving failing test.
- **Micro-Iterations**: Iterate in small, granular steps.
- **Test Integrity**: Never disable, alter, or remove failing tests just to force a build to pass.
