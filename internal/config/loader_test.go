package config_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validYAML = `
version: "1.0"
default_scenario: "checkout"

scenarios:
  checkout:
    type: "constant_vus"
    vus: 10
    ramp_up: "5s"
    run_period: "30s"
    ramp_down: "3s"
    vu_timeout: "2s"
    strict: "warn"
    params:
      base_url: "https://api.example.com"
    thresholds:
      - metric: "http_request_duration"
        stat: "p95"
        operator: "<"
        target: "200ms"
      - metric: "success_rate"
        stat: "rate"
        operator: ">"
        target: "0.995"

  payment:
    type: "arrival_rate"
    target_tps: 100
    max_vus: 300
    ramp_up: "10s"
    run_period: "1m"
    ramp_down: "5s"
    vu_timeout: "3s"
    thresholds:
      - metric: "payment_duration"
        stat: "p99"
        operator: "<"
        target: "500ms"
`

// AC-1.2.1: Valid YAML round-trips correctly
func TestValidYAMLRoundTrips(t *testing.T) {
	cfg, err := config.Load(strings.NewReader(validYAML))
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "1.0", cfg.Version)
	assert.Equal(t, "checkout", cfg.DefaultScenario)
	assert.Len(t, cfg.Scenarios, 2)

	// Verify checkout scenario.
	checkout, ok := cfg.Scenarios["checkout"]
	require.True(t, ok, "checkout scenario must exist")
	assert.Equal(t, config.ScenarioTypeConstantVUs, checkout.Type)
	assert.Equal(t, 10, checkout.VUs)
	assert.Equal(t, 5*time.Second, checkout.RampUp)
	assert.Equal(t, 30*time.Second, checkout.RunPeriod)
	assert.Equal(t, 3*time.Second, checkout.RampDown)
	assert.Equal(t, 2*time.Second, checkout.VUTimeout)
	assert.Equal(t, "warn", checkout.StrictMode)
	assert.Equal(t, "https://api.example.com", checkout.Params["base_url"])
	assert.Len(t, checkout.Thresholds, 2)

	// Verify payment scenario (arrival_rate).
	payment, ok := cfg.Scenarios["payment"]
	require.True(t, ok, "payment scenario must exist")
	assert.Equal(t, config.ScenarioTypeArrivalRate, payment.Type)
	assert.Equal(t, 100, payment.TargetTPS)
	assert.Equal(t, 300, payment.MaxVUs)
	assert.Equal(t, 10*time.Second, payment.RampUp)
	assert.Equal(t, 1*time.Minute, payment.RunPeriod)
	assert.Equal(t, 5*time.Second, payment.RampDown)
	assert.Equal(t, 3*time.Second, payment.VUTimeout)
}

func TestLoad_StrictMode_ParsedCorrectly(t *testing.T) {
	yaml := `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    strict: "warn"
`
	cfg, err := config.Load(strings.NewReader(yaml))
	require.NoError(t, err)
	assert.Equal(t, "warn", cfg.Scenarios["s1"].StrictMode)
}


// AC-1.2.2: Missing required field returns ValidationError
func TestMissingRequiredFieldReturnsValidationError(t *testing.T) {
	tests := []struct {
		name  string
		yaml  string
		field string
	}{
		{
			name: "missing version",
			yaml: `
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
`,
			field: "version",
		},
		{
			name: "missing scenarios",
			yaml: `
version: "1.0"
`,
			field: "scenarios",
		},
		{
			name: "missing type",
			yaml: `
version: "1.0"
scenarios:
  s1:
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
`,
			field: "scenarios.s1.type",
		},
		{
			name: "missing run_period",
			yaml: `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    vu_timeout: "1s"
`,
			field: "scenarios.s1.run_period",
		},
		{
			name: "negative vu_timeout",
			yaml: `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "-1s"
`,
			field: "scenarios.s1.vu_timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.Load(strings.NewReader(tt.yaml))
			require.Error(t, err)

			var valErr *config.ValidationError
			require.True(t, errors.As(err, &valErr), "expected *config.ValidationError, got %T: %v", err, err)
			assert.Equal(t, tt.field, valErr.Field)
		})
	}

	t.Run("omitted vu_timeout defaults to guard deadline", func(t *testing.T) {
		yaml := `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
`
		cfg, err := config.Load(strings.NewReader(yaml))
		require.NoError(t, err)
		assert.Equal(t, config.DefaultGuardDeadline, cfg.Scenarios["s1"].VUTimeout)
	})
}

// AC-1.2.3: Unknown scenario in default_scenario returns ValidationError
func TestUnknownDefaultScenarioReturnsValidationError(t *testing.T) {
	yaml := `
version: "1.0"
default_scenario: "nonexistent"

scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
`
	_, err := config.Load(strings.NewReader(yaml))
	require.Error(t, err)

	var valErr *config.ValidationError
	require.True(t, errors.As(err, &valErr), "expected *config.ValidationError, got %T", err)
	assert.Equal(t, "default_scenario", valErr.Field)
	assert.Contains(t, valErr.Message, "nonexistent")
}

// AC-1.2.4: arrival_rate without target_tps returns ValidationError
func TestArrivalRateWithoutTargetTPSReturnsValidationError(t *testing.T) {
	yaml := `
version: "1.0"

scenarios:
  s1:
    type: "arrival_rate"
    max_vus: 10
    run_period: "10s"
    vu_timeout: "1s"
`
	_, err := config.Load(strings.NewReader(yaml))
	require.Error(t, err)

	var valErr *config.ValidationError
	require.True(t, errors.As(err, &valErr), "expected *config.ValidationError, got %T", err)
	assert.Equal(t, "scenarios.s1.target_tps", valErr.Field)
}

// AC-1.2.5: constant_vus without vus returns ValidationError
func TestConstantVUsWithoutVUsReturnsValidationError(t *testing.T) {
	yaml := `
version: "1.0"

scenarios:
  s1:
    type: "constant_vus"
    run_period: "10s"
    vu_timeout: "1s"
`
	_, err := config.Load(strings.NewReader(yaml))
	require.Error(t, err)

	var valErr *config.ValidationError
	require.True(t, errors.As(err, &valErr), "expected *config.ValidationError, got %T", err)
	assert.Equal(t, "scenarios.s1.vus", valErr.Field)
}

// AC-1.2.6: threshold with invalid operator returns ValidationError
func TestThresholdWithInvalidOperatorReturnsValidationError(t *testing.T) {
	yaml := `
version: "1.0"

scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    thresholds:
      - metric: "latency"
        stat: "p95"
        operator: "=="
        target: "200ms"
`
	_, err := config.Load(strings.NewReader(yaml))
	require.Error(t, err)

	var valErr *config.ValidationError
	require.True(t, errors.As(err, &valErr), "expected *config.ValidationError, got %T", err)
	assert.Contains(t, valErr.Field, "operator")
}

// AC-1.2.7: threshold target "200ms" parses correctly for Duration stats
func TestThresholdDurationTargetParsesCorrectly(t *testing.T) {
	yaml := `
version: "1.0"

scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    thresholds:
      - metric: "latency"
        stat: "p95"
        operator: "<"
        target: "200ms"
`
	cfg, err := config.Load(strings.NewReader(yaml))
	require.NoError(t, err)

	th := cfg.Scenarios["s1"].Thresholds[0]
	assert.Equal(t, 200*time.Millisecond, th.TargetDuration)
	assert.Equal(t, float64(0), th.TargetFloat)
}

// AC-1.2.8: threshold target "0.005" parses correctly for rate stat
func TestThresholdRateTargetParsesCorrectly(t *testing.T) {
	yaml := `
version: "1.0"

scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    thresholds:
      - metric: "error_rate"
        stat: "rate"
        operator: "<"
        target: "0.005"
`
	cfg, err := config.Load(strings.NewReader(yaml))
	require.NoError(t, err)

	th := cfg.Scenarios["s1"].Thresholds[0]
	assert.InDelta(t, 0.005, th.TargetFloat, 1e-9)
	assert.Equal(t, time.Duration(0), th.TargetDuration)
}

// AC-1.2.9: threshold target "200ms" for "rate" stat returns ValidationError
func TestThresholdDurationTargetForRateStatReturnsValidationError(t *testing.T) {
	yaml := `
version: "1.0"

scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    thresholds:
      - metric: "error_rate"
        stat: "rate"
        operator: "<"
        target: "200ms"
`
	_, err := config.Load(strings.NewReader(yaml))
	require.Error(t, err)

	var valErr *config.ValidationError
	require.True(t, errors.As(err, &valErr), "expected *config.ValidationError, got %T", err)
	assert.Contains(t, valErr.Field, "target")
}

// Additional: arrival_rate without max_vus returns ValidationError
func TestArrivalRateWithoutMaxVUsReturnsValidationError(t *testing.T) {
	yaml := `
version: "1.0"

scenarios:
  s1:
    type: "arrival_rate"
    target_tps: 10
    run_period: "10s"
    vu_timeout: "1s"
`
	_, err := config.Load(strings.NewReader(yaml))
	require.Error(t, err)

	var valErr *config.ValidationError
	require.True(t, errors.As(err, &valErr), "expected *config.ValidationError, got %T", err)
	assert.Equal(t, "scenarios.s1.max_vus", valErr.Field)
}

// Additional: invalid YAML returns ConfigError
func TestInvalidYAMLReturnsConfigError(t *testing.T) {
	_, err := config.Load(strings.NewReader("{{invalid yaml"))
	require.Error(t, err)

	var cfgErr *config.ConfigError
	require.True(t, errors.As(err, &cfgErr), "expected *config.ConfigError, got %T", err)
}

// Additional: wrong version returns ValidationError
func TestWrongVersionReturnsValidationError(t *testing.T) {
	yaml := `
version: "2.0"

scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
`
	_, err := config.Load(strings.NewReader(yaml))
	require.Error(t, err)

	var valErr *config.ValidationError
	require.True(t, errors.As(err, &valErr), "expected *config.ValidationError, got %T", err)
	assert.Equal(t, "version", valErr.Field)
}

// Additional: threshold with invalid stat returns ValidationError
func TestThresholdWithInvalidStatReturnsValidationError(t *testing.T) {
	yaml := `
version: "1.0"

scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    thresholds:
      - metric: "latency"
        stat: "p100"
        operator: "<"
        target: "200ms"
`
	_, err := config.Load(strings.NewReader(yaml))
	require.Error(t, err)

	var valErr *config.ValidationError
	require.True(t, errors.As(err, &valErr), "expected *config.ValidationError, got %T", err)
	assert.Contains(t, valErr.Field, "stat")
}

// Additional: all valid duration stats are accepted
func TestAllDurationStatsAccepted(t *testing.T) {
	for _, stat := range []string{"p50", "p90", "p95", "p99", "mean", "max"} {
		t.Run(stat, func(t *testing.T) {
			assert.True(t, config.IsDurationStat(stat))

			yaml := `
version: "1.0"

scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    thresholds:
      - metric: "latency"
        stat: "` + stat + `"
        operator: "<"
        target: "100ms"
`
			cfg, err := config.Load(strings.NewReader(yaml))
			require.NoError(t, err)
			assert.Equal(t, 100*time.Millisecond, cfg.Scenarios["s1"].Thresholds[0].TargetDuration)
		})
	}
}

// Additional: all valid non-duration stats are accepted
func TestAllNonDurationStatsAccepted(t *testing.T) {
	for _, stat := range []string{"count", "rate", "value"} {
		t.Run(stat, func(t *testing.T) {
			assert.False(t, config.IsDurationStat(stat))

			yaml := `
version: "1.0"

scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    thresholds:
      - metric: "some_metric"
        stat: "` + stat + `"
        operator: ">="
        target: "42.5"
`
			cfg, err := config.Load(strings.NewReader(yaml))
			require.NoError(t, err)
			assert.InDelta(t, 42.5, cfg.Scenarios["s1"].Thresholds[0].TargetFloat, 1e-9)
		})
	}
}

// Additional: all valid operators are accepted
func TestAllValidOperatorsAccepted(t *testing.T) {
	for _, op := range []string{"<", "<=", ">", ">="} {
		t.Run(op, func(t *testing.T) {
			yaml := `
version: "1.0"

scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    thresholds:
      - metric: "latency"
        stat: "p95"
        operator: "` + op + `"
        target: "200ms"
`
			_, err := config.Load(strings.NewReader(yaml))
			require.NoError(t, err)
		})
	}
}

// Additional: optional fields default to zero values
func TestOptionalFieldsDefaultToZero(t *testing.T) {
	yaml := `
version: "1.0"

scenarios:
  s1:
    type: "constant_vus"
    vus: 5
    run_period: "10s"
    vu_timeout: "1s"
`
	cfg, err := config.Load(strings.NewReader(yaml))
	require.NoError(t, err)

	s := cfg.Scenarios["s1"]
	assert.Equal(t, time.Duration(0), s.RampUp)
	assert.Equal(t, time.Duration(0), s.RampDown)
	assert.Empty(t, s.Params)
	assert.Empty(t, s.Thresholds)
	assert.Empty(t, cfg.DefaultScenario)
}

// Additional: LoadFromFile with nonexistent file returns ConfigError
func TestLoadFromFileNotFoundReturnsConfigError(t *testing.T) {
	_, err := config.LoadFromFile("/nonexistent/path/vuhive.yaml")
	require.Error(t, err)

	var cfgErr *config.ConfigError
	require.True(t, errors.As(err, &cfgErr), "expected *config.ConfigError, got %T", err)
	assert.Equal(t, "/nonexistent/path/vuhive.yaml", cfgErr.Path)
}

func TestInteractionDelayConfigLoading(t *testing.T) {
	t.Run("valid fixed delay", func(t *testing.T) {
		yaml := `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    interaction_delay:
      type: "fixed"
      duration: "500ms"
`
		cfg, err := config.Load(strings.NewReader(yaml))
		require.NoError(t, err)
		require.NotNil(t, cfg.Scenarios["s1"].InteractionDelay)
		assert.Equal(t, "fixed", cfg.Scenarios["s1"].InteractionDelay.Type)
		assert.Equal(t, 500*time.Millisecond, cfg.Scenarios["s1"].InteractionDelay.Duration)
	})

	t.Run("valid range delay", func(t *testing.T) {
		yaml := `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    interaction_delay:
      type: "range"
      min: "200ms"
      max: "1s"
`
		cfg, err := config.Load(strings.NewReader(yaml))
		require.NoError(t, err)
		require.NotNil(t, cfg.Scenarios["s1"].InteractionDelay)
		assert.Equal(t, "range", cfg.Scenarios["s1"].InteractionDelay.Type)
		assert.Equal(t, 200*time.Millisecond, cfg.Scenarios["s1"].InteractionDelay.Min)
		assert.Equal(t, 1*time.Second, cfg.Scenarios["s1"].InteractionDelay.Max)
	})

	t.Run("valid expo delay", func(t *testing.T) {
		yaml := `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    interaction_delay:
      type: "expo"
      mean: "500ms"
      min: "100ms"
      max: "2s"
`
		cfg, err := config.Load(strings.NewReader(yaml))
		require.NoError(t, err)
		require.NotNil(t, cfg.Scenarios["s1"].InteractionDelay)
		assert.Equal(t, "expo", cfg.Scenarios["s1"].InteractionDelay.Type)
		assert.Equal(t, 500*time.Millisecond, cfg.Scenarios["s1"].InteractionDelay.Mean)
		assert.Equal(t, 100*time.Millisecond, cfg.Scenarios["s1"].InteractionDelay.Min)
		assert.Equal(t, 2*time.Second, cfg.Scenarios["s1"].InteractionDelay.Max)
	})

	t.Run("valid gaussian delay", func(t *testing.T) {
		yaml := `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    interaction_delay:
      type: "gaussian"
      mean: "500ms"
      std_dev: "50ms"
      min: "300ms"
      max: "700ms"
`
		cfg, err := config.Load(strings.NewReader(yaml))
		require.NoError(t, err)
		require.NotNil(t, cfg.Scenarios["s1"].InteractionDelay)
		assert.Equal(t, "gaussian", cfg.Scenarios["s1"].InteractionDelay.Type)
		assert.Equal(t, 500*time.Millisecond, cfg.Scenarios["s1"].InteractionDelay.Mean)
		assert.Equal(t, 50*time.Millisecond, cfg.Scenarios["s1"].InteractionDelay.StdDev)
		assert.Equal(t, 300*time.Millisecond, cfg.Scenarios["s1"].InteractionDelay.Min)
		assert.Equal(t, 700*time.Millisecond, cfg.Scenarios["s1"].InteractionDelay.Max)
	})

	t.Run("invalid delay type", func(t *testing.T) {
		yaml := `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    interaction_delay:
      type: "unknown_delay"
`
		_, err := config.Load(strings.NewReader(yaml))
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.interaction_delay.type", valErr.Field)
	})

	t.Run("invalid fixed delay without duration", func(t *testing.T) {
		yaml := `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    interaction_delay:
      type: "fixed"
`
		_, err := config.Load(strings.NewReader(yaml))
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.interaction_delay.duration", valErr.Field)
	})

	t.Run("invalid range delay max less than min", func(t *testing.T) {
		yaml := `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    interaction_delay:
      type: "range"
      min: "500ms"
      max: "100ms"
`
		_, err := config.Load(strings.NewReader(yaml))
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.interaction_delay.max", valErr.Field)
	})

	t.Run("invalid expo delay without mean", func(t *testing.T) {
		yaml := `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    interaction_delay:
      type: "expo"
`
		_, err := config.Load(strings.NewReader(yaml))
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.interaction_delay.mean", valErr.Field)
	})

	t.Run("invalid gaussian delay without std_dev", func(t *testing.T) {
		yaml := `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    interaction_delay:
      type: "gaussian"
      mean: "500ms"
`
		_, err := config.Load(strings.NewReader(yaml))
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.interaction_delay.std_dev", valErr.Field)
	})
}

func TestThinkTimeConfigLoading(t *testing.T) {
	t.Run("valid fixed think time", func(t *testing.T) {
		yaml := `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    think_time:
      type: "fixed"
      duration: "500ms"
`
		cfg, err := config.Load(strings.NewReader(yaml))
		require.NoError(t, err)
		sc := cfg.Scenarios["s1"]
		require.NotNil(t, sc.ThinkTime)
		assert.Equal(t, "fixed", sc.ThinkTime.Type)
		assert.Equal(t, 500*time.Millisecond, sc.ThinkTime.Duration)
	})

	t.Run("valid range think time and interaction delay together", func(t *testing.T) {
		yaml := `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    think_time:
      type: "range"
      min: "200ms"
      max: "1s"
    interaction_delay:
      type: "fixed"
      duration: "50ms"
`
		cfg, err := config.Load(strings.NewReader(yaml))
		require.NoError(t, err)
		sc := cfg.Scenarios["s1"]
		require.NotNil(t, sc.ThinkTime)
		assert.Equal(t, "range", sc.ThinkTime.Type)
		assert.Equal(t, 200*time.Millisecond, sc.ThinkTime.Min)
		assert.Equal(t, 1*time.Second, sc.ThinkTime.Max)

		require.NotNil(t, sc.InteractionDelay)
		assert.Equal(t, "fixed", sc.InteractionDelay.Type)
		assert.Equal(t, 50*time.Millisecond, sc.InteractionDelay.Duration)
	})

	t.Run("invalid think time unknown type", func(t *testing.T) {
		yaml := `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    think_time:
      type: "unknown"
`
		_, err := config.Load(strings.NewReader(yaml))
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.think_time.type", valErr.Field)
	})

	t.Run("invalid think time fixed missing duration", func(t *testing.T) {
		yaml := `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
    think_time:
      type: "fixed"
`
		_, err := config.Load(strings.NewReader(yaml))
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.think_time.duration", valErr.Field)
	})
}

func TestLoad_RampingVUs(t *testing.T) {
	t.Run("valid ramping_vus config loading", func(t *testing.T) {
		yaml := `
version: "1.0"
default_scenario: spike_test
scenarios:
  spike_test:
    type: ramping_vus
    vu_timeout: 2s
    ramp_down: 10s
    stages:
      - target: 10
        duration: 30s
      - target: 10
        duration: 1m
      - target: 50
        duration: 10s
      - target: 0
        duration: 30s
`
		cfg, err := config.Load(strings.NewReader(yaml))
		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.Equal(t, "1.0", cfg.Version)
		assert.Equal(t, "spike_test", cfg.DefaultScenario)

		sc, ok := cfg.Scenarios["spike_test"]
		require.True(t, ok)
		assert.Equal(t, config.ScenarioTypeRampingVUs, sc.Type)
		assert.Equal(t, 2*time.Second, sc.VUTimeout)
		assert.Equal(t, 10*time.Second, sc.RampDown)
		require.Len(t, sc.Stages, 4)

		assert.Equal(t, 10, sc.Stages[0].Target)
		assert.Equal(t, 30*time.Second, sc.Stages[0].Duration)

		assert.Equal(t, 10, sc.Stages[1].Target)
		assert.Equal(t, 1*time.Minute, sc.Stages[1].Duration)

		assert.Equal(t, 50, sc.Stages[2].Target)
		assert.Equal(t, 10*time.Second, sc.Stages[2].Duration)

		assert.Equal(t, 0, sc.Stages[3].Target)
		assert.Equal(t, 30*time.Second, sc.Stages[3].Duration)
	})
}

func TestLoad_DrainAndDrainPeriod(t *testing.T) {
	t.Run("valid drain duration in YAML", func(t *testing.T) {
		yaml := `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 5
    run_period: "10s"
    ramp_down: "5s"
    drain: "3s"
    vu_timeout: "1s"
`
		cfg, err := config.Load(strings.NewReader(yaml))
		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.Equal(t, 3*time.Second, cfg.Scenarios["s1"].Drain)
	})

	t.Run("valid drain_period alias duration in YAML", func(t *testing.T) {
		yaml := `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 5
    run_period: "10s"
    ramp_down: "5s"
    drain_period: "4s"
    vu_timeout: "1s"
`
		cfg, err := config.Load(strings.NewReader(yaml))
		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.Equal(t, 4*time.Second, cfg.Scenarios["s1"].Drain)
	})
}

func TestLoad_HTTPConfig(t *testing.T) {
	t.Run("valid full http config in YAML", func(t *testing.T) {
		yaml := `
version: "1.0"
scenarios:
  http_checkout:
    type: "constant_vus"
    vus: 50
    run_period: "1m"
    vu_timeout: "5s"
    http:
      base_url: "https://api.example.com"
      timeout: "5s"
      headers:
        Accept: "application/json"
        User-Agent: "vuhive/1.0"
      tls:
        insecure_skip_verify: true
      pool:
        max_idle_conns: 100
        max_idle_conns_per_host: 10
        idle_conn_timeout: "90s"
      detailed_timing: true
      metric_prefix: "vuhive.http.custom."
`
		cfg, err := config.Load(strings.NewReader(yaml))
		require.NoError(t, err)
		require.NotNil(t, cfg)

		sc, ok := cfg.Scenarios["http_checkout"]
		require.True(t, ok)
		require.NotNil(t, sc.HTTP)

		assert.Equal(t, "https://api.example.com", sc.HTTP.BaseURL)
		assert.Equal(t, 5*time.Second, sc.HTTP.Timeout)
		assert.Equal(t, "application/json", sc.HTTP.Headers["accept"])
		assert.Equal(t, "vuhive/1.0", sc.HTTP.Headers["user-agent"])
		assert.True(t, sc.HTTP.TLS.InsecureSkipVerify)
		assert.Equal(t, 100, sc.HTTP.Pool.MaxIdleConns)
		assert.Equal(t, 10, sc.HTTP.Pool.MaxIdleConnsPerHost)
		assert.Equal(t, 90*time.Second, sc.HTTP.Pool.IdleConnTimeout)
		assert.True(t, sc.HTTP.DetailedTiming)
		assert.Equal(t, "vuhive.http.custom.", sc.HTTP.MetricPrefix)
	})

	t.Run("scenario without http config has nil HTTP", func(t *testing.T) {
		yaml := `
version: "1.0"
scenarios:
  s1:
    type: "constant_vus"
    vus: 1
    run_period: "10s"
    vu_timeout: "1s"
`
		cfg, err := config.Load(strings.NewReader(yaml))
		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.Nil(t, cfg.Scenarios["s1"].HTTP)
	})
}





func TestLoad_HTTPClients(t *testing.T) {
	yamlInput := `
version: "1.0"
scenarios:
  multi_service:
    type: constant_vus
    vus: 10
    run_period: 30s
    vu_timeout: 5s
    http_clients:
      auth_service:
        base_url: "https://auth.example.com"
        timeout: 2s
        headers:
          X-Client-ID: "vuhive"
        pool:
          max_idle_conns: 20
        metric_prefix: "vuhive.http.auth."
      catalog_api:
        base_url: "https://catalog.example.com/api/v2"
        timeout: 5s
`
	cfg, err := config.Load(strings.NewReader(yamlInput))
	require.NoError(t, err)

	require.NotNil(t, cfg.Scenarios["multi_service"].HTTPClients)
	clients := cfg.Scenarios["multi_service"].HTTPClients
	require.Len(t, clients, 2)

	authClient := clients["auth_service"]
	require.NotNil(t, authClient)
	assert.Equal(t, "https://auth.example.com", authClient.BaseURL)
	assert.Equal(t, 2*time.Second, authClient.Timeout)
	assert.Equal(t, map[string]string{"x-client-id": "vuhive"}, authClient.Headers)
	assert.Equal(t, 20, authClient.Pool.MaxIdleConns)
	assert.Equal(t, "vuhive.http.auth.", authClient.MetricPrefix)

	catalogClient := clients["catalog_api"]
	require.NotNil(t, catalogClient)
	assert.Equal(t, "https://catalog.example.com/api/v2", catalogClient.BaseURL)
	assert.Equal(t, 5*time.Second, catalogClient.Timeout)
}
