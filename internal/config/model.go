// Package config provides YAML configuration loading, parsing, and validation
// for vuhive load test scenarios.
package config

import "time"

// ScenarioType defines the execution model for a scenario.
type ScenarioType string

const (
	// ScenarioTypeConstantVUs runs a fixed number of virtual user goroutines.
	ScenarioTypeConstantVUs ScenarioType = "constant_vus"

	// ScenarioTypeArrivalRate dispatches iterations at a target TPS using a token bucket.
	ScenarioTypeArrivalRate ScenarioType = "arrival_rate"

	// ScenarioTypeRampingVUs runs a dynamic number of virtual user goroutines across configured stages.
	ScenarioTypeRampingVUs ScenarioType = "ramping_vus"
)

// Config is the top-level configuration loaded from vuhive.yaml.
type Config struct {
	// Version must be "1.0".
	Version string

	// DefaultScenario is the name of the scenario to run when --scenario is not provided.
	// Must match a key in Scenarios if present.
	DefaultScenario string

	// Scenarios maps scenario names to their configurations.
	Scenarios map[string]ScenarioConfig
}

// StageConfig defines a single stage target VU count and duration for ramping_vus scenarios.
type StageConfig struct {
	// Target is the target VU count at the end of the stage.
	Target int

	// Duration is the duration of the stage.
	Duration time.Duration
}

// ScenarioConfig holds the configuration for a single load test scenario.
type ScenarioConfig struct {
	// Type is the execution model: "constant_vus", "arrival_rate", or "ramping_vus".
	Type ScenarioType

	// VUs is the number of virtual user goroutines (required for constant_vus).
	VUs int

	// TargetTPS is the target transactions per second (required for arrival_rate).
	TargetTPS int

	// MaxVUs is the hard cap on concurrent goroutines (required for arrival_rate).
	MaxVUs int

	// BurstBuffer is the bounded queue depth for arrival_rate token dispatch.
	// Absorbs transient worker availability fluctuations before dropping tokens.
	// When zero, a sensible default is computed automatically.
	BurstBuffer int

	// Stages defines the multi-stage ramping profile (required for ramping_vus).
	Stages []StageConfig

	// RampUp is the duration to linearly ramp up to the target level.
	RampUp time.Duration

	// RunPeriod is the steady-state execution duration (required for constant_vus and arrival_rate).
	RunPeriod time.Duration

	// RampDown is the duration for graceful ramp-down.
	RampDown time.Duration

	// Drain is the dedicated grace period allowing in-flight VUs to terminate cleanly.
	Drain time.Duration

	// MaxPreTestRetries is the maximum number of PreTest retry attempts upon initialization failure (default 3).
	MaxPreTestRetries int

	// MinReadyRatio is the minimum ratio of healthy initialized VUs required before starting the test (0.0 to 1.0).
	MinReadyRatio float64

	// StartupGracePeriod is the maximum grace period to wait for startup quorum readiness.
	StartupGracePeriod time.Duration

	// VUTimeout is the per-iteration context timeout (required).
	VUTimeout time.Duration

	// Params is an arbitrary key-value map available to test code via ScenarioContext.Param().
	Params map[string]string

	// InteractionDelay defines think time strategy between actions (via ctx.Sleep).
	InteractionDelay *InteractionDelayConfig

	// ThinkTime defines inter-iteration pacing delay automatically executed by the engine loop.
	ThinkTime *ThinkTimeConfig

	// Thresholds defines SLA assertions evaluated after the test run.
	Thresholds []ThresholdConfig

	// HTTP defines declarative HTTP client configuration.
	HTTP *HTTPConfig

	// HTTPClients defines named HTTP client configurations for multi-service scenarios.
	HTTPClients map[string]*HTTPConfig
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


// ThinkTimeConfig holds configuration for inter-iteration think time delays.
type ThinkTimeConfig struct {
	// Type is the strategy type: "fixed", "range", "expo", "gaussian".
	Type string

	// Duration is the static pause duration (used for "fixed").
	Duration time.Duration

	// Min is the minimum duration (used for "range", optional clamp for "expo" and "gaussian").
	Min time.Duration

	// Max is the maximum duration (used for "range", optional clamp for "expo" and "gaussian").
	Max time.Duration

	// Mean is the average duration (used for "expo" and "gaussian").
	Mean time.Duration

	// StdDev is the standard deviation (used for "gaussian").
	StdDev time.Duration
}

// InteractionDelayConfig holds configuration for in-iteration think time delays.
type InteractionDelayConfig = ThinkTimeConfig

// ThresholdConfig defines a single SLA threshold assertion.
type ThresholdConfig struct {
	// Metric is the exact metric name as recorded by the test developer.
	Metric string

	// Stat is the statistic to evaluate (p50, p90, p95, p99, mean, max, count, rate, value).
	Stat string

	// Operator is the comparison operator: <, <=, >, >=.
	Operator string

	// Target is the threshold value as a raw string.
	// Parsed as time.Duration for duration stats (p50, p90, p95, p99, mean, max),
	// or as float64 for non-duration stats (count, rate, value).
	Target string

	// OnNoData specifies the evaluation policy when no metric data is recorded during the test run.
	// Supported values: "zero", "fail", "pass", "ignore", "skip".
	// Defaults to "zero" for count and value stats, and "fail" for duration and rate stats.
	OnNoData string

	// AbortOnFail triggers early test termination if breached during execution.
	AbortOnFail bool

	// DelayAbortEval is a warm-up grace period before abort evaluation begins.
	DelayAbortEval time.Duration

	// TargetDuration is the parsed target when Stat is a duration stat.
	// Populated by validation; zero value if not applicable.
	TargetDuration time.Duration

	// TargetFloat is the parsed target when Stat is a non-duration stat.
	// Populated by validation; zero value if not applicable.
	TargetFloat float64
}

