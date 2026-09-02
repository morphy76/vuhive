package config

import (
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"
)

// durationStats is the set of stat values that require a time.Duration target.
var durationStats = map[string]bool{
	"p50":  true,
	"p90":  true,
	"p95":  true,
	"p99":  true,
	"mean": true,
	"max":  true,
}

// IsDurationStat reports whether the stat requires a time.Duration target.
func IsDurationStat(stat string) bool {
	return durationStats[stat]
}

// validStats is the complete set of supported stat values.
var validStats = map[string]bool{
	"p50":   true,
	"p90":   true,
	"p95":   true,
	"p99":   true,
	"mean":  true,
	"max":   true,
	"count": true,
	"rate":  true,
	"value": true,
}

// validOperators is the set of supported comparison operators.
var validOperators = map[string]bool{
	"<":  true,
	"<=": true,
	">":  true,
	">=": true,
}

// validOnNoData is the set of supported on_no_data strategy values.
var validOnNoData = map[string]bool{
	"":       true,
	"zero":   true,
	"fail":   true,
	"pass":   true,
	"ignore": true,
	"skip":   true,
}

// InferredMetricKind returns the metric kind string ("duration", "counter", "rate", "gauge") for a stat.
func InferredMetricKind(stat string) string {
	if IsDurationStat(stat) {
		return "duration"
	}
	switch stat {
	case "count":
		return "counter"
	case "rate":
		return "rate"
	case "value":
		return "gauge"
	default:
		return "unknown"
	}
}

// DelayValidatorFunc validates a delay configuration against strategy bounds.
type DelayValidatorFunc func(prefix string, delay *ThinkTimeConfig) error

var (
	delayValidatorsMu sync.RWMutex
	delayValidators   = map[string]DelayValidatorFunc{
		"fixed":    validateFixedDelay,
		"range":    validateRangeDelay,
		"expo":     validateExpoDelay,
		"gaussian": validateGaussianDelay,
	}
)

// RegisterDelayValidator registers a validator function for a specific delay strategy type.
func RegisterDelayValidator(strategy string, fn DelayValidatorFunc) {
	delayValidatorsMu.Lock()
	defer delayValidatorsMu.Unlock()
	delayValidators[strategy] = fn
}

func getDelayValidator(strategy string) (DelayValidatorFunc, bool) {
	delayValidatorsMu.RLock()
	defer delayValidatorsMu.RUnlock()
	fn, ok := delayValidators[strategy]
	return fn, ok
}

// validateDelayConfig checks a delay configuration against strategy bounds.
func validateDelayConfig(delayPrefix string, delay *ThinkTimeConfig) error {
	validator, exists := getDelayValidator(delay.Type)
	if !exists {
		return &ValidationError{
			Field:   delayPrefix + ".type",
			Message: fmt.Sprintf("must be one of {fixed, range, expo, gaussian}, got %q", delay.Type),
		}
	}
	return validator(delayPrefix, delay)
}

func validateFixedDelay(prefix string, delay *ThinkTimeConfig) error {
	if delay.Duration <= 0 {
		return &ValidationError{
			Field:   prefix + ".duration",
			Message: "must be > 0 for fixed delay",
		}
	}
	return nil
}

func validateRangeDelay(prefix string, delay *ThinkTimeConfig) error {
	if delay.Min < 0 {
		return &ValidationError{
			Field:   prefix + ".min",
			Message: "must be >= 0 for range delay",
		}
	}
	if delay.Max <= 0 {
		return &ValidationError{
			Field:   prefix + ".max",
			Message: "must be > 0 for range delay",
		}
	}
	if delay.Max < delay.Min {
		return &ValidationError{
			Field:   prefix + ".max",
			Message: "must be >= min for range delay",
		}
	}
	return nil
}

func validateExpoDelay(prefix string, delay *ThinkTimeConfig) error {
	if delay.Mean <= 0 {
		return &ValidationError{
			Field:   prefix + ".mean",
			Message: "must be > 0 for expo delay",
		}
	}
	if delay.Min < 0 {
		return &ValidationError{
			Field:   prefix + ".min",
			Message: "must be >= 0",
		}
	}
	if delay.Max < 0 {
		return &ValidationError{
			Field:   prefix + ".max",
			Message: "must be >= 0",
		}
	}
	if delay.Min > 0 && delay.Max > 0 && delay.Max < delay.Min {
		return &ValidationError{
			Field:   prefix + ".max",
			Message: "must be >= min for expo delay",
		}
	}
	return nil
}

func validateGaussianDelay(prefix string, delay *ThinkTimeConfig) error {
	if delay.Mean <= 0 {
		return &ValidationError{
			Field:   prefix + ".mean",
			Message: "must be > 0 for gaussian delay",
		}
	}
	if delay.StdDev <= 0 {
		return &ValidationError{
			Field:   prefix + ".std_dev",
			Message: "must be > 0 for gaussian delay",
		}
	}
	if delay.Min < 0 {
		return &ValidationError{
			Field:   prefix + ".min",
			Message: "must be >= 0",
		}
	}
	if delay.Max < 0 {
		return &ValidationError{
			Field:   prefix + ".max",
			Message: "must be >= 0",
		}
	}
	if delay.Min > 0 && delay.Max > 0 && delay.Max < delay.Min {
		return &ValidationError{
			Field:   prefix + ".max",
			Message: "must be >= min for gaussian delay",
		}
	}
	return nil
}

// Validate checks all semantic invariants on a parsed Config.
// Returns a *ValidationError on the first violation found.
func Validate(cfg *Config) error {
	// Version must be "1.0".
	if cfg.Version != "1.0" {
		return &ValidationError{
			Field:   "version",
			Message: fmt.Sprintf("must be %q, got %q", "1.0", cfg.Version),
		}
	}

	// Must have at least one scenario.
	if len(cfg.Scenarios) == 0 {
		return &ValidationError{
			Field:   "scenarios",
			Message: "at least one scenario must be defined",
		}
	}

	// default_scenario must match a key in scenarios if present.
	if cfg.DefaultScenario != "" {
		if _, ok := cfg.Scenarios[cfg.DefaultScenario]; !ok {
			return &ValidationError{
				Field:   "default_scenario",
				Message: fmt.Sprintf("references unknown scenario %q", cfg.DefaultScenario),
			}
		}
	}

	// Validate each scenario.
	for name, sc := range cfg.Scenarios {
		if err := validateScenario(name, &sc); err != nil {
			return err
		}
		// Write back the validated scenario (threshold parsed values).
		cfg.Scenarios[name] = sc
	}

	return nil
}

// validateScenario checks a single scenario configuration.
func validateScenario(name string, sc *ScenarioConfig) error {
	prefix := fmt.Sprintf("scenarios.%s", name)

	// Type is required and must be one of the known types.
	switch sc.Type {
	case ScenarioTypeConstantVUs:
		if sc.VUs <= 0 {
			return &ValidationError{
				Field:   prefix + ".vus",
				Message: "must be > 0 for constant_vus scenario type",
			}
		}
		if sc.RunPeriod <= 0 {
			return &ValidationError{
				Field:   prefix + ".run_period",
				Message: "must be > 0",
			}
		}
	case ScenarioTypeArrivalRate:
		if sc.TargetTPS <= 0 {
			return &ValidationError{
				Field:   prefix + ".target_tps",
				Message: "must be > 0 for arrival_rate scenario type",
			}
		}
		if sc.MaxVUs <= 0 {
			return &ValidationError{
				Field:   prefix + ".max_vus",
				Message: "must be > 0 for arrival_rate scenario type",
			}
		}
		if sc.RunPeriod <= 0 {
			return &ValidationError{
				Field:   prefix + ".run_period",
				Message: "must be > 0",
			}
		}
		// Little's Law capacity validation: RequiredVUs = ceil(TargetTPS × VUTimeout_seconds).
		if sc.VUTimeout > 0 {
			vuTimeoutSec := sc.VUTimeout.Seconds()
			requiredVUs := int(math.Ceil(float64(sc.TargetTPS) * vuTimeoutSec))
			if sc.MaxVUs < requiredVUs {
				return &ValidationError{
					Field: prefix + ".max_vus",
					Message: fmt.Sprintf(
						"insufficient for target throughput: max_vus=%d but Little's Law requires at least %d (ceil(%d × %.3fs)); maximum achievable TPS is %.1f",
						sc.MaxVUs, requiredVUs, sc.TargetTPS, vuTimeoutSec,
						float64(sc.MaxVUs)/vuTimeoutSec,
					),
				}
			}
		}
		// burst_buffer must be >= 0.
		if sc.BurstBuffer < 0 {
			return &ValidationError{
				Field:   prefix + ".burst_buffer",
				Message: "must be >= 0",
			}
		}
	case ScenarioTypeRampingVUs:
		if len(sc.Stages) == 0 {
			return &ValidationError{
				Field:   prefix + ".stages",
				Message: "at least one stage must be defined for ramping_vus scenario type",
			}
		}
		for i, st := range sc.Stages {
			stPrefix := fmt.Sprintf("%s.stages[%d]", prefix, i)
			if st.Target < 0 {
				return &ValidationError{
					Field:   stPrefix + ".target",
					Message: "must be >= 0",
				}
			}
			if st.Duration <= 0 {
				return &ValidationError{
					Field:   stPrefix + ".duration",
					Message: "must be > 0",
				}
			}
		}
	default:
		return &ValidationError{
			Field:   prefix + ".type",
			Message: fmt.Sprintf("must be %q, %q, or %q, got %q", ScenarioTypeConstantVUs, ScenarioTypeArrivalRate, ScenarioTypeRampingVUs, sc.Type),
		}
	}

	// Guard deadline defaulting and vu_timeout validation.
	if sc.VUTimeout < 0 {
		return &ValidationError{
			Field:   prefix + ".vu_timeout",
			Message: "must be >= 0",
		}
	}
	if sc.VUTimeout == 0 && !sc.AllowUnboundedIterations {
		sc.VUTimeout = DefaultGuardDeadline
	}

	// ramp_up must be >= 0 (it defaults to 0).
	if sc.RampUp < 0 {
		return &ValidationError{
			Field:   prefix + ".ramp_up",
			Message: "must be >= 0",
		}
	}

	// ramp_down must be >= 0 (it defaults to 0).
	if sc.RampDown < 0 {
		return &ValidationError{
			Field:   prefix + ".ramp_down",
			Message: "must be >= 0",
		}
	}

	// drain must be >= 0 (it defaults to 0).
	if sc.Drain < 0 {
		return &ValidationError{
			Field:   prefix + ".drain",
			Message: "must be >= 0",
		}
	}

	// max_pretest_retries must be >= 0.
	if sc.MaxPreTestRetries < 0 {
		return &ValidationError{
			Field:   prefix + ".max_pretest_retries",
			Message: "must be >= 0",
		}
	}

	// min_ready_ratio must be between 0.0 and 1.0.
	if sc.MinReadyRatio < 0.0 || sc.MinReadyRatio > 1.0 {
		return &ValidationError{
			Field:   prefix + ".min_ready_ratio",
			Message: "must be between 0.0 and 1.0",
		}
	}

	// startup_grace_period must be >= 0.
	if sc.StartupGracePeriod < 0 {
		return &ValidationError{
			Field:   prefix + ".startup_grace_period",
			Message: "must be >= 0",
		}
	}

	// watchdog_stall_threshold must be >= 0.
	if sc.WatchdogStallThreshold < 0 {
		return &ValidationError{
			Field:   prefix + ".watchdog_stall_threshold",
			Message: "must be >= 0",
		}
	}

	// watchdog_interval must be >= 0.
	if sc.WatchdogInterval < 0 {
		return &ValidationError{
			Field:   prefix + ".watchdog_interval",
			Message: "must be >= 0",
		}
	}


	// Validate params: keys and values must be non-empty strings.
	for k, v := range sc.Params {
		if k == "" {
			return &ValidationError{
				Field:   prefix + ".params",
				Message: "param keys must be non-empty strings",
			}
		}
		if v == "" {
			return &ValidationError{
				Field:   prefix + ".params." + k,
				Message: "param values must be non-empty strings",
			}
		}
	}

	// Validate interaction_delay if specified.
	if sc.InteractionDelay != nil {
		if err := validateInteractionDelay(prefix, sc.InteractionDelay); err != nil {
			return err
		}
	}

	// Validate think_time if specified.
	if sc.ThinkTime != nil {
		if err := validateThinkTime(prefix, sc.ThinkTime); err != nil {
			return err
		}
	}

	// Validate http if specified.
	if sc.HTTP != nil {
		if err := validateHTTPConfig(fmt.Sprintf("%s.http", prefix), sc.HTTP); err != nil {
			return err
		}
	}

	for clientName, clientCfg := range sc.HTTPClients {
		if clientName == "" {
			return &ValidationError{
				Field:   prefix + ".http_clients",
				Message: "client names must be non-empty strings",
			}
		}
		if clientCfg == nil {
			return &ValidationError{
				Field:   fmt.Sprintf("%s.http_clients.%s", prefix, clientName),
				Message: "client configuration must not be null",
			}
		}
		if err := validateHTTPConfig(fmt.Sprintf("%s.http_clients.%s", prefix, clientName), clientCfg); err != nil {
			return err
		}
	}

	// Validate thresholds and detect conflicting inferred metric kinds.
	metricKinds := make(map[string]string)
	for i := range sc.Thresholds {
		if err := validateThreshold(prefix, i, &sc.Thresholds[i]); err != nil {
			return err
		}
		kind := InferredMetricKind(sc.Thresholds[i].Stat)
		if existingKind, ok := metricKinds[sc.Thresholds[i].Metric]; ok && existingKind != kind {
			return &ValidationError{
				Field:   fmt.Sprintf("%s.thresholds[%d].metric", prefix, i),
				Message: fmt.Sprintf("conflicting metric kind for metric %q: inferred %s, previously inferred %s", sc.Thresholds[i].Metric, kind, existingKind),
			}
		}
		metricKinds[sc.Thresholds[i].Metric] = kind
	}

	return nil
}

// validateInteractionDelay checks the interaction delay configuration.
func validateInteractionDelay(prefix string, delay *InteractionDelayConfig) error {
	return validateDelayConfig(fmt.Sprintf("%s.interaction_delay", prefix), delay)
}

// validateThinkTime checks the inter-iteration think time configuration.
func validateThinkTime(prefix string, tt *ThinkTimeConfig) error {
	return validateDelayConfig(fmt.Sprintf("%s.think_time", prefix), tt)
}

// validateHTTPConfig checks the declarative HTTP client configuration.
func validateHTTPConfig(httpPrefix string, httpCfg *HTTPConfig) error {
	if httpCfg.Timeout < 0 {
		return &ValidationError{
			Field:   httpPrefix + ".timeout",
			Message: "must be >= 0",
		}
	}
	if httpCfg.Pool.MaxIdleConns < 0 {
		return &ValidationError{
			Field:   httpPrefix + ".pool.max_idle_conns",
			Message: "must be >= 0",
		}
	}
	if httpCfg.Pool.MaxIdleConnsPerHost < 0 {
		return &ValidationError{
			Field:   httpPrefix + ".pool.max_idle_conns_per_host",
			Message: "must be >= 0",
		}
	}
	if httpCfg.Pool.IdleConnTimeout < 0 {
		return &ValidationError{
			Field:   httpPrefix + ".pool.idle_conn_timeout",
			Message: "must be >= 0",
		}
	}
	return nil
}

// validateThreshold checks a single threshold configuration and parses its target value.
func validateThreshold(prefix string, idx int, th *ThresholdConfig) error {
	thPrefix := fmt.Sprintf("%s.thresholds[%d]", prefix, idx)

	// metric is required.
	if th.Metric == "" {
		return &ValidationError{
			Field:   thPrefix + ".metric",
			Message: "must not be empty",
		}
	}

	// stat must be one of the known values.
	if !validStats[th.Stat] {
		return &ValidationError{
			Field:   thPrefix + ".stat",
			Message: fmt.Sprintf("must be one of {p50, p90, p95, p99, mean, max, count, rate, value}, got %q", th.Stat),
		}
	}

	// operator must be one of the known values.
	if !validOperators[th.Operator] {
		return &ValidationError{
			Field:   thPrefix + ".operator",
			Message: fmt.Sprintf("must be one of {<, <=, >, >=}, got %q", th.Operator),
		}
	}

	// target is required.
	if th.Target == "" {
		return &ValidationError{
			Field:   thPrefix + ".target",
			Message: "must not be empty",
		}
	}

	// on_no_data must be one of the known strategies if specified.
	if !validOnNoData[th.OnNoData] {
		return &ValidationError{
			Field:   thPrefix + ".on_no_data",
			Message: fmt.Sprintf("must be one of {zero, fail, pass, ignore, skip}, got %q", th.OnNoData),
		}
	}

	// Populate default on_no_data if not specified.
	if th.OnNoData == "" {
		if th.Stat == "count" || th.Stat == "value" {
			th.OnNoData = "zero"
		} else {
			th.OnNoData = "fail"
		}
	}

	// delay_abort_eval must be >= 0
	if th.DelayAbortEval < 0 {
		return &ValidationError{
			Field:   thPrefix + ".delay_abort_eval",
			Message: "must be >= 0",
		}
	}

	// Parse target based on stat type.
	if IsDurationStat(th.Stat) {
		d, err := time.ParseDuration(th.Target)
		if err != nil {
			return &ValidationError{
				Field:   thPrefix + ".target",
				Message: fmt.Sprintf("cannot parse %q as duration for stat %q: %s", th.Target, th.Stat, err),
			}
		}
		th.TargetDuration = d
	} else {
		f, err := strconv.ParseFloat(th.Target, 64)
		if err != nil {
			return &ValidationError{
				Field:   thPrefix + ".target",
				Message: fmt.Sprintf("cannot parse %q as float64 for stat %q: %s", th.Target, th.Stat, err),
			}
		}
		th.TargetFloat = f
	}

	return nil
}
