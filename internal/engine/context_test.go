package engine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/engine"
	"github.com/morphy76/vuhive/internal/log"
	"github.com/morphy76/vuhive/internal/metric"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)


// AC-1.12.6: ctx.Sleep() halts for generated duration and aborts immediately on ctx.Done()
func TestScenarioContextSleep(t *testing.T) {
	logger, metrics := newTestDeps()

	t.Run("explicit duration pauses for expected time", func(t *testing.T) {
		var durations []time.Duration
		var mu sync.Mutex

		scenario := engine.Scenario{
			RunVU: func(ctx engine.VUContext) error {
				start := time.Now()
				err := ctx.Sleep(50 * time.Millisecond)
				if err == nil {
					mu.Lock()
					durations = append(durations, time.Since(start))
					mu.Unlock()
				}
				return err
			},
		}

		cfg := config.ScenarioConfig{
			Type:      config.ScenarioTypeConstantVUs,
			VUs:       1,
			RunPeriod: 100 * time.Millisecond,
			VUTimeout: 1 * time.Second,
		}

		exec := engine.NewExecutor("sleep_explicit", scenario, cfg, logger, metrics)
		err := exec.Execute(context.Background())
		require.NoError(t, err)

		mu.Lock()
		defer mu.Unlock()
		require.NotEmpty(t, durations)
		for _, d := range durations {
			assert.GreaterOrEqual(t, d, 45*time.Millisecond)
		}
	})


	t.Run("aborts immediately when context is cancelled", func(t *testing.T) {
		var sleepErr error
		var duration time.Duration

		ctx, cancel := context.WithCancel(context.Background())

		scenario := engine.Scenario{
			RunVU: func(sc engine.VUContext) error {
				go func() {
					time.Sleep(20 * time.Millisecond)
					cancel()
				}()
				start := time.Now()
				sleepErr = sc.Sleep(1 * time.Second)
				duration = time.Since(start)
				return sleepErr
			},
		}

		cfg := config.ScenarioConfig{
			Type:      config.ScenarioTypeConstantVUs,
			VUs:       1,
			RunPeriod: 2 * time.Second,
			VUTimeout: 5 * time.Second,
		}

		exec := engine.NewExecutor("sleep_cancel", scenario, cfg, logger, metrics)
		_ = exec.Execute(ctx)

		require.Error(t, sleepErr)
		assert.Equal(t, context.Canceled, sleepErr)
		assert.Less(t, duration, 200*time.Millisecond, "sleep should have aborted quickly on context cancellation")
	})

	t.Run("uses configured interaction_delay when called with no arguments", func(t *testing.T) {
		var durations []time.Duration
		var mu sync.Mutex

		scenario := engine.Scenario{
			RunVU: func(ctx engine.VUContext) error {
				start := time.Now()
				err := ctx.Sleep()
				if err == nil {
					mu.Lock()
					durations = append(durations, time.Since(start))
					mu.Unlock()
				}
				return err
			},
		}

		cfg := config.ScenarioConfig{
			Type:      config.ScenarioTypeConstantVUs,
			VUs:       1,
			RunPeriod: 150 * time.Millisecond,
			VUTimeout: 1 * time.Second,
			InteractionDelay: &config.InteractionDelayConfig{
				Type:     "fixed",
				Duration: 60 * time.Millisecond,
			},
		}

		exec := engine.NewExecutor("sleep_configured", scenario, cfg, logger, metrics)
		err := exec.Execute(context.Background())
		require.NoError(t, err)

		mu.Lock()
		defer mu.Unlock()
		require.NotEmpty(t, durations, "must have completed at least one sleep")
		for _, d := range durations {
			assert.GreaterOrEqual(t, d, 55*time.Millisecond)
		}
	})


	t.Run("returns nil immediately when no delay is configured and no arg passed", func(t *testing.T) {
		var sleptDuration time.Duration

		scenario := engine.Scenario{
			RunVU: func(ctx engine.VUContext) error {
				start := time.Now()
				err := ctx.Sleep()
				sleptDuration = time.Since(start)
				return err
			},
		}

		cfg := config.ScenarioConfig{
			Type:      config.ScenarioTypeConstantVUs,
			VUs:       1,
			RunPeriod: 50 * time.Millisecond,
			VUTimeout: 1 * time.Second,
		}

		exec := engine.NewExecutor("sleep_none", scenario, cfg, logger, metrics)
		err := exec.Execute(context.Background())
		require.NoError(t, err)

		assert.Less(t, sleptDuration, 10*time.Millisecond)
	})

	t.Run("uses configured think_time when called with no arguments", func(t *testing.T) {
		var durations []time.Duration
		var mu sync.Mutex

		scenario := engine.Scenario{
			RunVU: func(ctx engine.VUContext) error {
				start := time.Now()
				err := ctx.Sleep()
				if err == nil {
					mu.Lock()
					durations = append(durations, time.Since(start))
					mu.Unlock()
				}
				return err
			},
		}

		cfg := config.ScenarioConfig{
			Type:      config.ScenarioTypeConstantVUs,
			VUs:       1,
			RunPeriod: 150 * time.Millisecond,
			VUTimeout: 1 * time.Second,
			ThinkTime: &config.ThinkTimeConfig{
				Type:     "fixed",
				Duration: 60 * time.Millisecond,
			},
		}

		exec := engine.NewExecutor("sleep_think_time_configured", scenario, cfg, logger, metrics)
		err := exec.Execute(context.Background())
		require.NoError(t, err)

		mu.Lock()
		defer mu.Unlock()
		require.NotEmpty(t, durations, "must have completed at least one sleep")
		for _, d := range durations {
			assert.GreaterOrEqual(t, d, 55*time.Millisecond)
		}
	})

	t.Run("zero or negative explicit duration returns nil immediately", func(t *testing.T) {
		var sleptDuration time.Duration

		scenario := engine.Scenario{
			RunVU: func(ctx engine.VUContext) error {
				start := time.Now()
				err1 := ctx.Sleep(0)
				err2 := ctx.Sleep(-50 * time.Millisecond)
				sleptDuration = time.Since(start)
				if err1 != nil {
					return err1
				}
				return err2
			},
		}

		cfg := config.ScenarioConfig{
			Type:      config.ScenarioTypeConstantVUs,
			VUs:       1,
			RunPeriod: 50 * time.Millisecond,
			VUTimeout: 1 * time.Second,
		}

		exec := engine.NewExecutor("sleep_zero_negative", scenario, cfg, logger, metrics)
		err := exec.Execute(context.Background())
		require.NoError(t, err)

		assert.Less(t, sleptDuration, 10*time.Millisecond)
	})

	t.Run("pre-cancelled context returns ctx.Err() immediately", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		sCtx := engine.NewScenarioContext(ctx, 1, 0, config.ScenarioConfig{}, "test", nil, logger, metrics)
		err := sCtx.Sleep(1 * time.Second)

		require.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

func TestScenarioContext_InterfaceSegregation(t *testing.T) {
	logger, metrics := newTestDeps()
	cfg := config.ScenarioConfig{
		Params: map[string]string{
			"env":     "staging",
			"retries": "3",
			"timeout": "500ms",
		},
	}
	state := map[string]any{"token": "jwt_secret_123"}

	sCtx := engine.NewScenarioContext(context.Background(), 5, 12, cfg, "order_scenario", state, logger, metrics)

	t.Run("satisfies ExecutionIdentity", func(t *testing.T) {
		var id engine.ExecutionIdentity = sCtx
		assert.Equal(t, int64(5), id.VUID())
		assert.Equal(t, int64(12), id.Iteration())
		assert.Equal(t, "order_scenario", id.ScenarioName())
	})

	t.Run("satisfies ConfigProvider", func(t *testing.T) {
		var cp engine.ConfigProvider = sCtx
		assert.Equal(t, "staging", cp.Param("env"))
		assert.Equal(t, "", cp.Param("missing"))
		assert.Equal(t, 3, cp.ParamInt("retries", 1))
		assert.Equal(t, 10, cp.ParamInt("missing_int", 10))
		assert.Equal(t, 500*time.Millisecond, cp.ParamDuration("timeout", time.Second))
		assert.Equal(t, 2*time.Second, cp.ParamDuration("missing_dur", 2*time.Second))
	})

	t.Run("satisfies StateProvider", func(t *testing.T) {
		var sp engine.StateProvider = sCtx
		assert.Equal(t, "jwt_secret_123", sp.GlobalState("token"))
		assert.Nil(t, sp.GlobalState("missing_key"))
	})

	t.Run("satisfies ObservabilityProvider", func(t *testing.T) {
		var op engine.ObservabilityProvider = sCtx
		assert.NotNil(t, op.Log())
		assert.NotNil(t, op.Metrics())
	})

	t.Run("satisfies WorkflowController", func(t *testing.T) {
		var wc engine.WorkflowController = sCtx
		passed := wc.Check("is_ok", func() string { return "" })
		assert.True(t, passed)

		failed := wc.Check("is_fail", func() string { return "check error" })
		assert.False(t, failed)
	})

	t.Run("satisfies role-specific context interfaces", func(t *testing.T) {
		var setupCtx engine.SetupContext = sCtx
		assert.Equal(t, "staging", setupCtx.Param("env"))
		assert.NotNil(t, setupCtx.Log())

		var vuCtx engine.VUContext = sCtx
		assert.Equal(t, int64(5), vuCtx.VUID())
		assert.Equal(t, "jwt_secret_123", vuCtx.GlobalState("token"))

		var teardownCtx engine.TeardownContext = sCtx
		assert.Equal(t, "staging", teardownCtx.Param("env"))
		assert.Equal(t, "jwt_secret_123", teardownCtx.GlobalState("token"))

		var summaryCtx engine.SummaryContext = sCtx
		assert.Equal(t, "staging", summaryCtx.Param("env"))
		assert.NotNil(t, summaryCtx.Log())
	})
}

func TestScenarioContext_ParamParsingWarnings(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, zerolog.DebugLevel)
	metrics := metric.NewStore()

	cfg := config.ScenarioConfig{
		Params: map[string]string{
			"valid_int":   "42",
			"invalid_int": "abc",
			"valid_dur":   "250ms",
			"invalid_dur": "not_a_duration",
			"empty_param": "",
		},
	}

	sCtx := engine.NewScenarioContext(context.Background(), 1, 0, cfg, "test_scenario", nil, logger, metrics)

	t.Run("ParamInt with valid int does not log warning", func(t *testing.T) {
		buf.Reset()
		val := sCtx.ParamInt("valid_int", 10)
		assert.Equal(t, 42, val)
		assert.Empty(t, buf.String())
	})

	t.Run("ParamInt with missing key does not log warning", func(t *testing.T) {
		buf.Reset()
		val := sCtx.ParamInt("missing_key", 10)
		assert.Equal(t, 10, val)
		assert.Empty(t, buf.String())
	})

	t.Run("ParamInt with empty value does not log warning", func(t *testing.T) {
		buf.Reset()
		val := sCtx.ParamInt("empty_param", 10)
		assert.Equal(t, 10, val)
		assert.Empty(t, buf.String())
	})

	t.Run("ParamInt with invalid value returns default and logs warning", func(t *testing.T) {
		buf.Reset()
		val := sCtx.ParamInt("invalid_int", 99)
		assert.Equal(t, 99, val)

		var logEntry map[string]any
		err := json.Unmarshal(buf.Bytes(), &logEntry)
		require.NoError(t, err, "log should be valid json: %s", buf.String())
		assert.Equal(t, "warn", logEntry["level"])
		assert.Equal(t, "invalid_int", logEntry["key"])
		assert.Equal(t, "abc", logEntry["value"])
		assert.Equal(t, float64(99), logEntry["default"])
		assert.NotEmpty(t, logEntry["error"])
		assert.Contains(t, logEntry["message"], "failed to parse param as integer")
	})

	t.Run("ParamDuration with valid duration does not log warning", func(t *testing.T) {
		buf.Reset()
		val := sCtx.ParamDuration("valid_dur", time.Second)
		assert.Equal(t, 250*time.Millisecond, val)
		assert.Empty(t, buf.String())
	})

	t.Run("ParamDuration with missing key does not log warning", func(t *testing.T) {
		buf.Reset()
		val := sCtx.ParamDuration("missing_key", time.Second)
		assert.Equal(t, time.Second, val)
		assert.Empty(t, buf.String())
	})

	t.Run("ParamDuration with empty value does not log warning", func(t *testing.T) {
		buf.Reset()
		val := sCtx.ParamDuration("empty_param", time.Second)
		assert.Equal(t, time.Second, val)
		assert.Empty(t, buf.String())
	})

	t.Run("ParamDuration with invalid value returns default and logs warning", func(t *testing.T) {
		buf.Reset()
		val := sCtx.ParamDuration("invalid_dur", 5*time.Second)
		assert.Equal(t, 5*time.Second, val)

		var logEntry map[string]any
		err := json.Unmarshal(buf.Bytes(), &logEntry)
		require.NoError(t, err, "log should be valid json: %s", buf.String())
		assert.Equal(t, "warn", logEntry["level"])
		assert.Equal(t, "invalid_dur", logEntry["key"])
		assert.Equal(t, "not_a_duration", logEntry["value"])
		assert.Equal(t, float64(5000), logEntry["default"])
		assert.NotEmpty(t, logEntry["error"])
		assert.Contains(t, logEntry["message"], "failed to parse param as duration")
	})

	t.Run("Nil logger does not panic on parse failure", func(t *testing.T) {
		nilLoggerCtx := engine.NewScenarioContext(context.Background(), 1, 0, cfg, "test_scenario", nil, nil, metrics)
		assert.NotPanics(t, func() {
			valInt := nilLoggerCtx.ParamInt("invalid_int", 123)
			assert.Equal(t, 123, valInt)
			valDur := nilLoggerCtx.ParamDuration("invalid_dur", 3*time.Second)
			assert.Equal(t, 3*time.Second, valDur)
		})
	})
}

func TestScenarioContext_HTTPConfig(t *testing.T) {
	logger, metrics := newTestDeps()

	t.Run("HTTPConfig populated from ScenarioConfig", func(t *testing.T) {
		cfg := config.ScenarioConfig{
			Type:      config.ScenarioTypeConstantVUs,
			VUs:       1,
			RunPeriod: 1 * time.Second,
			VUTimeout: 1 * time.Second,
			HTTP: &config.HTTPConfig{
				BaseURL: "https://api.example.com",
				Timeout: 5 * time.Second,
				Headers: map[string]string{"Accept": "application/json"},
				TLS:     config.TLSConfig{InsecureSkipVerify: true},
				Pool: config.HTTPPoolConfig{
					MaxIdleConns:        50,
					MaxIdleConnsPerHost: 10,
					IdleConnTimeout:     60 * time.Second,
				},
				DetailedTiming: true,
				MetricPrefix:   "custom.http.",
			},
		}

		sCtx := engine.NewScenarioContext(context.Background(), 1, 0, cfg, "http_test", nil, logger, metrics)
		httpCfg := sCtx.HTTPConfig()

		assert.Equal(t, "https://api.example.com", httpCfg.BaseURL)
		assert.Equal(t, 5*time.Second, httpCfg.Timeout)
		assert.Equal(t, "application/json", httpCfg.Headers["Accept"])
		assert.True(t, httpCfg.TLS.InsecureSkipVerify)
		assert.Equal(t, 50, httpCfg.Pool.MaxIdleConns)
		assert.Equal(t, 10, httpCfg.Pool.MaxIdleConnsPerHost)
		assert.Equal(t, 60*time.Second, httpCfg.Pool.IdleConnTimeout)
		assert.True(t, httpCfg.DetailedTiming)
		assert.Equal(t, "custom.http.", httpCfg.MetricPrefix)
	})

	t.Run("HTTPConfig empty when ScenarioConfig has no HTTP", func(t *testing.T) {
		cfg := config.ScenarioConfig{
			Type:      config.ScenarioTypeConstantVUs,
			VUs:       1,
			RunPeriod: 1 * time.Second,
			VUTimeout: 1 * time.Second,
		}

		sCtx := engine.NewScenarioContext(context.Background(), 1, 0, cfg, "no_http_test", nil, logger, metrics)
		httpCfg := sCtx.HTTPConfig()
		assert.Empty(t, httpCfg.BaseURL)
		assert.Equal(t, time.Duration(0), httpCfg.Timeout)
	})
}

func TestScenarioContext_CheckConvenienceMethods(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, zerolog.WarnLevel)
	metrics := metric.NewStore()

	sCtx := engine.NewScenarioContext(context.Background(), 1, 0, config.ScenarioConfig{}, "test_scenario", nil, logger, metrics)

	t.Run("CheckEqual primitive types", func(t *testing.T) {
		pass := sCtx.CheckEqual("eq_pass", 200, 200)
		fail := sCtx.CheckEqual("eq_fail", 404, 200)

		assert.True(t, pass)
		assert.False(t, fail)
		assert.Contains(t, buf.String(), `"check":"eq_fail"`)
		assert.Contains(t, buf.String(), `expected 200, got 404`)
	})

	t.Run("CheckEqual slice/map deep equality", func(t *testing.T) {
		pass := sCtx.CheckEqual("slice_pass", []string{"a", "b"}, []string{"a", "b"})
		fail := sCtx.CheckEqual("slice_fail", []string{"a"}, []string{"b"})

		assert.True(t, pass)
		assert.False(t, fail)
	})

	t.Run("CheckTrue", func(t *testing.T) {
		pass := sCtx.CheckTrue("true_pass", 10 > 5)
		fail := sCtx.CheckTrue("true_fail", 5 > 10, "5 is not greater than 10")

		assert.True(t, pass)
		assert.False(t, fail)
		assert.Contains(t, buf.String(), `"check":"true_fail"`)
		assert.Contains(t, buf.String(), `5 is not greater than 10`)
	})

	t.Run("CheckNoError", func(t *testing.T) {
		pass := sCtx.CheckNoError("noerr_pass", nil)
		fail := sCtx.CheckNoError("noerr_fail", assert.AnError)

		assert.True(t, pass)
		assert.False(t, fail)
		assert.Contains(t, buf.String(), `"check":"noerr_fail"`)
		assert.Contains(t, buf.String(), `unexpected error`)
	})

	t.Run("nil metrics and nil logger fallback", func(t *testing.T) {
		bareCtx := engine.NewScenarioContext(context.Background(), 1, 0, config.ScenarioConfig{}, "test_scenario", nil, nil, nil)
		assert.True(t, bareCtx.CheckEqual("eq", 1, 1))
		assert.False(t, bareCtx.CheckEqual("eq_fail", 1, 2))
		assert.True(t, bareCtx.CheckTrue("t", true))
		assert.False(t, bareCtx.CheckTrue("t_fail", false))
		assert.True(t, bareCtx.CheckNoError("e", nil))
		assert.False(t, bareCtx.CheckNoError("e_fail", assert.AnError))
	})
}






