package vuhive

import (
	"context"
	"time"
)

// ExecutionIdentity provides execution identity attributes (VU ID, iteration index, and scenario name).
type ExecutionIdentity interface {
	// VUID returns the unique 1-indexed identifier of the calling Virtual User.
	VUID() int64

	// Iteration returns the current zero-indexed iteration number of the VU.
	Iteration() int64

	// ScenarioName returns the name of the executing scenario.
	ScenarioName() string
}

// ConfigProvider provides access to scenario configuration parameters.
type ConfigProvider interface {
	// Param retrieves a string value from the scenario's params map. Returns "" if absent.
	Param(key string) string

	// ParamInt retrieves a params value parsed as int. Returns defaultValue if key is absent.
	// If the value is present but cannot be parsed as an integer, a Warn-level log is emitted
	// and defaultValue is returned.
	ParamInt(key string, defaultValue int) int

	// ParamDuration retrieves a params value parsed as time.Duration. Returns defaultValue if key is absent.
	// If the value is present but cannot be parsed as a duration, a Warn-level log is emitted
	// and defaultValue is returned.
	ParamDuration(key string, defaultValue time.Duration) time.Duration

	// HTTPConfig retrieves the declarative HTTP client configuration for the scenario.
	HTTPConfig() HTTPConfig

	// HTTPClients retrieves the named HTTP client configurations map.
	HTTPClients() map[string]HTTPConfig
}

// HTTPConfig holds declarative configuration for the HTTP client module.
type HTTPConfig struct {
	// BaseURL is the base URL prepended to relative request paths.
	BaseURL string

	// Timeout is the per-request timeout.
	Timeout time.Duration

	// Headers contains default headers sent with every request.
	Headers map[string]string

	// TLS contains TLS transport configuration.
	TLS TLSConfig

	// Pool contains connection pool configuration.
	Pool HTTPPoolConfig

	// DetailedTiming enables httptrace per-phase timing metrics.
	DetailedTiming bool

	// MetricPrefix overrides the default "vuhive.http." metric name prefix.
	MetricPrefix string
}

// TLSConfig holds TLS transport settings.
type TLSConfig struct {
	// InsecureSkipVerify disables TLS certificate verification.
	InsecureSkipVerify bool
}

// HTTPPoolConfig holds connection pool settings.
type HTTPPoolConfig struct {
	// MaxIdleConns is the maximum number of idle (keep-alive) connections across all hosts.
	MaxIdleConns int

	// MaxIdleConnsPerHost is the maximum number of idle connections per host.
	MaxIdleConnsPerHost int

	// IdleConnTimeout is the maximum duration an idle connection remains in the pool.
	IdleConnTimeout time.Duration
}

// StateProvider provides read-only access to global scenario state returned by Setup.
// Note: Global state is shallow-copied by the framework. Complex or nested mutable values
// (such as slices, maps, or pointers) must be immutable or protected with explicit synchronization.
type StateProvider interface {
	// GlobalState returns a value from the scenario's global state returned by Setup.
	// Returns nil if key is not found in the global state map.
	GlobalState(key string) any
}

// ObservabilityProvider provides access to structured logging and metric collection.
type ObservabilityProvider interface {
	// Log returns the structured logger scoped to the current VU execution.
	Log() Logger

	// Metrics returns the thread-safe metrics collector handle for recording telemetry.
	Metrics() MetricsCollector
}

// WorkflowController provides workflow execution controls such as delays and inline assertions.
type WorkflowController interface {
	// Sleep pauses execution for the configured think time or an explicitly provided duration.
	// When called with no arguments (Sleep()), it pauses for the think_time duration generated
	// by the scenario's configured think time strategy.
	// When called with an explicit duration (Sleep(d)), it pauses for that exact duration.
	// Respects context cancellation and returns ctx.Err() if cancelled during sleep.
	Sleep(d ...time.Duration) error

	// Check evaluates an inline assertion function fn and records the result under the given name.
	// Increments vuhive.checks.passed if fn returns an empty string, or vuhive.checks.failed if fn
	// returns a failure reason. Returns true if the check passed.
	Check(name string, fn CheckFunc) bool

	// CheckEqual evaluates actual == expected under the given check name.
	// Increments vuhive.checks.passed if actual equals expected, or vuhive.checks.failed if they differ.
	// Returns true if the check passed.
	CheckEqual(name string, actual, expected any) bool

	// CheckTrue evaluates condition under the given check name.
	// Increments vuhive.checks.passed if condition is true, or vuhive.checks.failed if false.
	// An optional failureReason can be provided to customize the failure message.
	// Returns true if the check passed.
	CheckTrue(name string, condition bool, failureReason ...string) bool

	// CheckNoError evaluates that err == nil under the given check name.
	// Increments vuhive.checks.passed if err is nil, or vuhive.checks.failed if err is not nil.
	// Returns true if the check passed.
	CheckNoError(name string, err error) bool

	// Group executes fn within a named transaction boundary.
	// Duration is automatically recorded to "vuhive.group.<name>.duration".
	// Nested groups are allowed; names are concatenated with "::".
	Group(name string, fn func(ctx VUContext) error) error
}


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

