package config_test

import (
	"errors"
	"testing"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsDurationStat(t *testing.T) {
	durationStats := []string{"p50", "p90", "p95", "p99", "mean", "max"}
	for _, stat := range durationStats {
		t.Run(stat+" is duration stat", func(t *testing.T) {
			assert.True(t, config.IsDurationStat(stat))
		})
	}

	nonDurationStats := []string{"count", "rate", "value", "unknown", ""}
	for _, stat := range nonDurationStats {
		t.Run(stat+" is not duration stat", func(t *testing.T) {
			assert.False(t, config.IsDurationStat(stat))
		})
	}
}

func TestValidate_StrictMode_ValidValues(t *testing.T) {
	for _, mode := range []string{"", "off", "warn", "fatal"} {
		t.Run("mode_"+mode, func(t *testing.T) {
			cfg := &config.Config{
				Version: "1.0",
				Scenarios: map[string]config.ScenarioConfig{
					"s1": {
						Type:       config.ScenarioTypeConstantVUs,
						VUs:        1,
						RunPeriod:  10 * time.Second,
						StrictMode: mode,
					},
				},
			}
			err := config.Validate(cfg)
			require.NoError(t, err)
		})
	}
}

func TestValidate_StrictMode_InvalidValue(t *testing.T) {
	cfg := &config.Config{
		Version: "1.0",
		Scenarios: map[string]config.ScenarioConfig{
			"s1": {
				Type:       config.ScenarioTypeConstantVUs,
				VUs:        1,
				RunPeriod:  10 * time.Second,
				StrictMode: "invalid",
			},
		},
	}
	err := config.Validate(cfg)
	require.Error(t, err)
	var valErr *config.ValidationError
	require.True(t, errors.As(err, &valErr))
	assert.Equal(t, "scenarios.s1.strict", valErr.Field)
	assert.Contains(t, valErr.Message, "must be one of {off, warn, fatal}")
}


func TestValidate_CustomDelayValidatorRegistry(t *testing.T) {
	// Register a custom delay strategy validator.
	config.RegisterDelayValidator("custom_step", func(prefix string, delay *config.ThinkTimeConfig) error {
		if delay.Duration <= 0 {
			return &config.ValidationError{
				Field:   prefix + ".duration",
				Message: "must be > 0 for custom_step delay",
			}
		}
		return nil
	})

	t.Run("custom delay valid", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					InteractionDelay: &config.InteractionDelayConfig{
						Type:     "custom_step",
						Duration: 100 * time.Millisecond,
					},
				},
			},
		}
		err := config.Validate(cfg)
		require.NoError(t, err)
	})

	t.Run("custom delay invalid", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					InteractionDelay: &config.InteractionDelayConfig{
						Type:     "custom_step",
						Duration: 0,
					},
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.interaction_delay.duration", valErr.Field)
		assert.Contains(t, valErr.Message, "must be > 0 for custom_step delay")
	})
}

func TestValidate_DelayStrategyEdgeCases(t *testing.T) {
	baseCfg := func(delay *config.ThinkTimeConfig) *config.Config {
		return &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:             config.ScenarioTypeConstantVUs,
					VUs:              1,
					RunPeriod:        10 * time.Second,
					VUTimeout:        1 * time.Second,
					InteractionDelay: delay,
				},
			},
		}
	}

	tests := []struct {
		name      string
		delay     *config.ThinkTimeConfig
		wantField string
	}{
		{
			name: "range delay min negative",
			delay: &config.ThinkTimeConfig{
				Type: "range",
				Min:  -1 * time.Second,
				Max:  1 * time.Second,
			},
			wantField: "scenarios.s1.interaction_delay.min",
		},
		{
			name: "range delay max zero",
			delay: &config.ThinkTimeConfig{
				Type: "range",
				Min:  0,
				Max:  0,
			},
			wantField: "scenarios.s1.interaction_delay.max",
		},
		{
			name: "expo delay min negative",
			delay: &config.ThinkTimeConfig{
				Type: "expo",
				Mean: 500 * time.Millisecond,
				Min:  -1 * time.Millisecond,
			},
			wantField: "scenarios.s1.interaction_delay.min",
		},
		{
			name: "expo delay max negative",
			delay: &config.ThinkTimeConfig{
				Type: "expo",
				Mean: 500 * time.Millisecond,
				Max:  -1 * time.Millisecond,
			},
			wantField: "scenarios.s1.interaction_delay.max",
		},
		{
			name: "expo delay max less than min",
			delay: &config.ThinkTimeConfig{
				Type: "expo",
				Mean: 500 * time.Millisecond,
				Min:  500 * time.Millisecond,
				Max:  100 * time.Millisecond,
			},
			wantField: "scenarios.s1.interaction_delay.max",
		},
		{
			name: "gaussian delay mean zero",
			delay: &config.ThinkTimeConfig{
				Type:   "gaussian",
				Mean:   0,
				StdDev: 10 * time.Millisecond,
			},
			wantField: "scenarios.s1.interaction_delay.mean",
		},
		{
			name: "gaussian delay stddev zero",
			delay: &config.ThinkTimeConfig{
				Type:   "gaussian",
				Mean:   100 * time.Millisecond,
				StdDev: 0,
			},
			wantField: "scenarios.s1.interaction_delay.std_dev",
		},
		{
			name: "gaussian delay min negative",
			delay: &config.ThinkTimeConfig{
				Type:   "gaussian",
				Mean:   100 * time.Millisecond,
				StdDev: 10 * time.Millisecond,
				Min:    -10 * time.Millisecond,
			},
			wantField: "scenarios.s1.interaction_delay.min",
		},
		{
			name: "gaussian delay max negative",
			delay: &config.ThinkTimeConfig{
				Type:   "gaussian",
				Mean:   100 * time.Millisecond,
				StdDev: 10 * time.Millisecond,
				Max:    -10 * time.Millisecond,
			},
			wantField: "scenarios.s1.interaction_delay.max",
		},
		{
			name: "gaussian delay max less than min",
			delay: &config.ThinkTimeConfig{
				Type:   "gaussian",
				Mean:   100 * time.Millisecond,
				StdDev: 10 * time.Millisecond,
				Min:    200 * time.Millisecond,
				Max:    50 * time.Millisecond,
			},
			wantField: "scenarios.s1.interaction_delay.max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseCfg(tt.delay)
			err := config.Validate(cfg)
			require.Error(t, err)
			var valErr *config.ValidationError
			require.True(t, errors.As(err, &valErr))
			assert.Equal(t, tt.wantField, valErr.Field)
		})
	}
}

func TestValidate_ScenarioEdgeCases(t *testing.T) {
	t.Run("negative ramp_up", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					RampUp:    -1 * time.Second,
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.ramp_up", valErr.Field)
	})

	t.Run("negative ramp_down", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					RampDown:  -1 * time.Second,
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.ramp_down", valErr.Field)
	})

	t.Run("negative drain", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					Drain:     -1 * time.Second,
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.drain", valErr.Field)
	})


	t.Run("empty param key", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					Params:    map[string]string{"": "val"},
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.params", valErr.Field)
	})

	t.Run("empty param value", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					Params:    map[string]string{"key": ""},
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.params.key", valErr.Field)
	})

	t.Run("threshold metric empty", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					Thresholds: []config.ThresholdConfig{
						{Metric: "", Stat: "p95", Operator: "<", Target: "100ms"},
					},
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.thresholds[0].metric", valErr.Field)
	})

	t.Run("threshold target empty", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					Thresholds: []config.ThresholdConfig{
						{Metric: "m", Stat: "p95", Operator: "<", Target: ""},
					},
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.thresholds[0].target", valErr.Field)
	})

	t.Run("threshold delay_abort_eval negative", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					Thresholds: []config.ThresholdConfig{
						{Metric: "m", Stat: "p95", Operator: "<", Target: "100ms", DelayAbortEval: -1 * time.Second},
					},
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.thresholds[0].delay_abort_eval", valErr.Field)
	})

	t.Run("threshold on_no_data valid strategies", func(t *testing.T) {
		for _, strategy := range []string{"zero", "fail", "pass", "ignore", "skip"} {
			cfg := &config.Config{
				Version: "1.0",
				Scenarios: map[string]config.ScenarioConfig{
					"s1": {
						Type:      config.ScenarioTypeConstantVUs,
						VUs:       1,
						RunPeriod: 10 * time.Second,
						VUTimeout: 1 * time.Second,
						Thresholds: []config.ThresholdConfig{
							{Metric: "m", Stat: "count", Operator: "<=", Target: "0", OnNoData: strategy},
						},
					},
				},
			}
			err := config.Validate(cfg)
			require.NoError(t, err, "strategy %q should be valid", strategy)
			assert.Equal(t, strategy, cfg.Scenarios["s1"].Thresholds[0].OnNoData)
		}
	})

	t.Run("threshold on_no_data invalid strategy", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					Thresholds: []config.ThresholdConfig{
						{Metric: "m", Stat: "count", Operator: "<=", Target: "0", OnNoData: "invalid_strategy"},
					},
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.thresholds[0].on_no_data", valErr.Field)
	})

	t.Run("threshold on_no_data defaults populated", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					Thresholds: []config.ThresholdConfig{
						{Metric: "c", Stat: "count", Operator: "<=", Target: "0"},
						{Metric: "g", Stat: "value", Operator: "<=", Target: "10"},
						{Metric: "d", Stat: "p95", Operator: "<", Target: "100ms"},
						{Metric: "r", Stat: "rate", Operator: ">=", Target: "0.99"},
					},
				},
			},
		}
		err := config.Validate(cfg)
		require.NoError(t, err)
		assert.Equal(t, "zero", cfg.Scenarios["s1"].Thresholds[0].OnNoData)
		assert.Equal(t, "zero", cfg.Scenarios["s1"].Thresholds[1].OnNoData)
		assert.Equal(t, "fail", cfg.Scenarios["s1"].Thresholds[2].OnNoData)
		assert.Equal(t, "fail", cfg.Scenarios["s1"].Thresholds[3].OnNoData)
	})

	t.Run("threshold conflicting metric types", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					Thresholds: []config.ThresholdConfig{
						{Metric: "m", Stat: "p95", Operator: "<", Target: "100ms"},
						{Metric: "m", Stat: "count", Operator: "<=", Target: "0"},
					},
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.thresholds[1].metric", valErr.Field)
	})
}

func TestValidate_HTTPConfig(t *testing.T) {
	t.Run("negative timeout", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					HTTP: &config.HTTPConfig{
						Timeout: -1 * time.Second,
					},
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.http.timeout", valErr.Field)
	})

	t.Run("negative pool max_idle_conns", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					HTTP: &config.HTTPConfig{
						Pool: config.HTTPPoolConfig{
							MaxIdleConns: -1,
						},
					},
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.http.pool.max_idle_conns", valErr.Field)
	})

	t.Run("negative pool max_idle_conns_per_host", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					HTTP: &config.HTTPConfig{
						Pool: config.HTTPPoolConfig{
							MaxIdleConnsPerHost: -5,
						},
					},
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.http.pool.max_idle_conns_per_host", valErr.Field)
	})

	t.Run("negative pool idle_conn_timeout", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					HTTP: &config.HTTPConfig{
						Pool: config.HTTPPoolConfig{
							IdleConnTimeout: -10 * time.Second,
						},
					},
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.http.pool.idle_conn_timeout", valErr.Field)
	})

	t.Run("valid http config", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					HTTP: &config.HTTPConfig{
						BaseURL: "https://api.example.com",
						Timeout: 5 * time.Second,
						Pool: config.HTTPPoolConfig{
							MaxIdleConns:        100,
							MaxIdleConnsPerHost: 10,
							IdleConnTimeout:     90 * time.Second,
						},
					},
				},
			},
		}
		err := config.Validate(cfg)
		require.NoError(t, err)
	})
}


func TestValidate_HTTPClients(t *testing.T) {
	t.Run("empty name", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					HTTPClients: map[string]*config.HTTPConfig{
						"": {
							Timeout: 1 * time.Second,
						},
					},
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.http_clients", valErr.Field)
	})

	t.Run("nil config", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					HTTPClients: map[string]*config.HTTPConfig{
						"auth": nil,
					},
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.http_clients.auth", valErr.Field)
	})

	t.Run("negative timeout", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					HTTPClients: map[string]*config.HTTPConfig{
						"auth": {
							Timeout: -1 * time.Second,
						},
					},
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.http_clients.auth.timeout", valErr.Field)
	})

	t.Run("valid http_clients", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       1,
					RunPeriod: 10 * time.Second,
					VUTimeout: 1 * time.Second,
					HTTPClients: map[string]*config.HTTPConfig{
						"auth": {
							BaseURL: "https://auth.example.com",
							Timeout: 2 * time.Second,
						},
					},
				},
			},
		}
		err := config.Validate(cfg)
		require.NoError(t, err)
	})
}

func TestValidate_ArrivalRate_InsufficientMaxVUs(t *testing.T) {
	// Little's Law: RequiredVUs = ceil(TargetTPS * VUTimeout_seconds)
	// TargetTPS=100, VUTimeout=2s → RequiredVUs = ceil(100 * 2) = 200
	// MaxVUs=10 is insufficient.
	cfg := &config.Config{
		Version: "1.0",
		Scenarios: map[string]config.ScenarioConfig{
			"s1": {
				Type:      config.ScenarioTypeArrivalRate,
				TargetTPS: 100,
				MaxVUs:    10,
				RunPeriod: 10 * time.Second,
				VUTimeout: 2 * time.Second,
			},
		},
	}
	err := config.Validate(cfg)
	require.Error(t, err)
	var valErr *config.ValidationError
	require.True(t, errors.As(err, &valErr))
	assert.Equal(t, "scenarios.s1.max_vus", valErr.Field)
	assert.Contains(t, valErr.Message, "200")
}

func TestValidate_ArrivalRate_SufficientMaxVUs(t *testing.T) {
	// TargetTPS=100, VUTimeout=2s → RequiredVUs = 200
	// MaxVUs=200 is exactly sufficient.
	cfg := &config.Config{
		Version: "1.0",
		Scenarios: map[string]config.ScenarioConfig{
			"s1": {
				Type:      config.ScenarioTypeArrivalRate,
				TargetTPS: 100,
				MaxVUs:    200,
				RunPeriod: 10 * time.Second,
				VUTimeout: 2 * time.Second,
			},
		},
	}
	err := config.Validate(cfg)
	require.NoError(t, err)
}

func TestValidate_ArrivalRate_ExactBoundaryMaxVUs(t *testing.T) {
	// TargetTPS=10, VUTimeout=1s → RequiredVUs = ceil(10 * 1) = 10
	// MaxVUs=10 is exactly sufficient.
	cfg := &config.Config{
		Version: "1.0",
		Scenarios: map[string]config.ScenarioConfig{
			"s1": {
				Type:      config.ScenarioTypeArrivalRate,
				TargetTPS: 10,
				MaxVUs:    10,
				RunPeriod: 10 * time.Second,
				VUTimeout: 1 * time.Second,
			},
		},
	}
	err := config.Validate(cfg)
	require.NoError(t, err)
}

func TestValidate_ArrivalRate_FractionalCeiling(t *testing.T) {
	// TargetTPS=3, VUTimeout=500ms → RequiredVUs = ceil(3 * 0.5) = ceil(1.5) = 2
	// MaxVUs=1 is insufficient.
	cfg := &config.Config{
		Version: "1.0",
		Scenarios: map[string]config.ScenarioConfig{
			"s1": {
				Type:      config.ScenarioTypeArrivalRate,
				TargetTPS: 3,
				MaxVUs:    1,
				RunPeriod: 10 * time.Second,
				VUTimeout: 500 * time.Millisecond,
			},
		},
	}
	err := config.Validate(cfg)
	require.Error(t, err)
	var valErr *config.ValidationError
	require.True(t, errors.As(err, &valErr))
	assert.Equal(t, "scenarios.s1.max_vus", valErr.Field)
	assert.Contains(t, valErr.Message, "2")
}

func TestValidate_ArrivalRate_NegativeBurstBuffer(t *testing.T) {
	cfg := &config.Config{
		Version: "1.0",
		Scenarios: map[string]config.ScenarioConfig{
			"s1": {
				Type:        config.ScenarioTypeArrivalRate,
				TargetTPS:   10,
				MaxVUs:      10,
				RunPeriod:   10 * time.Second,
				VUTimeout:   1 * time.Second,
				BurstBuffer: -1,
			},
		},
	}
	err := config.Validate(cfg)
	require.Error(t, err)
	var valErr *config.ValidationError
	require.True(t, errors.As(err, &valErr))
	assert.Equal(t, "scenarios.s1.burst_buffer", valErr.Field)
}

func TestValidate_ArrivalRate_ValidBurstBuffer(t *testing.T) {
	cfg := &config.Config{
		Version: "1.0",
		Scenarios: map[string]config.ScenarioConfig{
			"s1": {
				Type:        config.ScenarioTypeArrivalRate,
				TargetTPS:   10,
				MaxVUs:      10,
				RunPeriod:   10 * time.Second,
				VUTimeout:   1 * time.Second,
				BurstBuffer: 50,
			},
		},
	}
	err := config.Validate(cfg)
	require.NoError(t, err)
}

func TestValidate_SupervisorAndStartupQuorum(t *testing.T) {
	t.Run("valid supervisor and quorum settings", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:               config.ScenarioTypeConstantVUs,
					VUs:                10,
					RunPeriod:          10 * time.Second,
					VUTimeout:          1 * time.Second,
					MaxPreTestRetries:  3,
					MinReadyRatio:      0.9,
					StartupGracePeriod: 5 * time.Second,
				},
			},
		}
		err := config.Validate(cfg)
		require.NoError(t, err)
	})

	t.Run("negative max_pretest_retries invalid", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:              config.ScenarioTypeConstantVUs,
					VUs:               10,
					RunPeriod:         10 * time.Second,
					VUTimeout:         1 * time.Second,
					MaxPreTestRetries: -1,
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.max_pretest_retries", valErr.Field)
	})

	t.Run("min_ready_ratio below zero invalid", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:          config.ScenarioTypeConstantVUs,
					VUs:           10,
					RunPeriod:     10 * time.Second,
					VUTimeout:     1 * time.Second,
					MinReadyRatio: -0.1,
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.min_ready_ratio", valErr.Field)
	})

	t.Run("min_ready_ratio above 1.0 invalid", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:          config.ScenarioTypeConstantVUs,
					VUs:           10,
					RunPeriod:     10 * time.Second,
					VUTimeout:     1 * time.Second,
					MinReadyRatio: 1.1,
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.min_ready_ratio", valErr.Field)
	})

	t.Run("negative startup_grace_period invalid", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:               config.ScenarioTypeConstantVUs,
					VUs:                10,
					RunPeriod:          10 * time.Second,
					VUTimeout:          1 * time.Second,
					StartupGracePeriod: -1 * time.Second,
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.startup_grace_period", valErr.Field)
	})

	t.Run("vu_timeout defaulted to guard deadline when unset", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       10,
					RunPeriod: 10 * time.Second,
					VUTimeout: 0,
				},
			},
		}
		err := config.Validate(cfg)
		require.NoError(t, err)
		assert.Equal(t, config.DefaultGuardDeadline, cfg.Scenarios["s1"].VUTimeout)
	})

	t.Run("vu_timeout zero allowed when allow_unbounded_iterations is true", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:                     config.ScenarioTypeConstantVUs,
					VUs:                      10,
					RunPeriod:                10 * time.Second,
					VUTimeout:                0,
					AllowUnboundedIterations: true,
				},
			},
		}
		err := config.Validate(cfg)
		require.NoError(t, err)
		assert.Equal(t, time.Duration(0), cfg.Scenarios["s1"].VUTimeout)
	})

	t.Run("negative vu_timeout invalid", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:      config.ScenarioTypeConstantVUs,
					VUs:       10,
					RunPeriod: 10 * time.Second,
					VUTimeout: -1 * time.Second,
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.vu_timeout", valErr.Field)
	})

	t.Run("negative watchdog_stall_threshold invalid", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:                   config.ScenarioTypeConstantVUs,
					VUs:                    10,
					RunPeriod:              10 * time.Second,
					VUTimeout:              1 * time.Second,
					WatchdogStallThreshold: -1 * time.Second,
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.watchdog_stall_threshold", valErr.Field)
	})

	t.Run("negative watchdog_interval invalid", func(t *testing.T) {
		cfg := &config.Config{
			Version: "1.0",
			Scenarios: map[string]config.ScenarioConfig{
				"s1": {
					Type:             config.ScenarioTypeConstantVUs,
					VUs:              10,
					RunPeriod:        10 * time.Second,
					VUTimeout:        1 * time.Second,
					WatchdogInterval: -1 * time.Second,
				},
			},
		}
		err := config.Validate(cfg)
		require.Error(t, err)
		var valErr *config.ValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, "scenarios.s1.watchdog_interval", valErr.Field)
	})
}

