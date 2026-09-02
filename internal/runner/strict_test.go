package runner_test

import (
	"testing"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/metric"
	"github.com/morphy76/vuhive/internal/runner"
	"github.com/stretchr/testify/assert"
)

func TestResolveStrictMode_CLIStrictFatalTakesPrecedence(t *testing.T) {
	result := runner.ResolveStrictMode(true, true, "warn")
	assert.Equal(t, config.StrictModeFatal, result)
}

func TestResolveStrictMode_CLIStrictOverridesYAML(t *testing.T) {
	result := runner.ResolveStrictMode(true, false, "off")
	assert.Equal(t, config.StrictModeWarn, result)
}

func TestResolveStrictMode_YAMLFallback(t *testing.T) {
	result := runner.ResolveStrictMode(false, false, "fatal")
	assert.Equal(t, config.StrictModeFatal, result)
}

func TestResolveStrictMode_DisabledByDefault(t *testing.T) {
	result := runner.ResolveStrictMode(false, false, "")
	assert.Equal(t, "", result)
}

func TestResolveStrictMode_YAMLOffDisablesStrict(t *testing.T) {
	result := runner.ResolveStrictMode(false, false, "off")
	assert.Equal(t, "", result)
}

func TestEvaluateStrictDiagnostics_UnusedParams(t *testing.T) {
	cfg := config.ScenarioConfig{
		Params: map[string]string{"used": "v", "unused": "v"},
	}
	store := metric.NewStore()
	diags := runner.EvaluateStrictDiagnostics(cfg, []string{"unused"}, store)
	assert.Len(t, diags, 1)
	assert.Equal(t, "unused_param", diags[0].Kind)
	assert.Contains(t, diags[0].Message, "unused")
}

func TestEvaluateStrictDiagnostics_UnmatchedThreshold(t *testing.T) {
	cfg := config.ScenarioConfig{
		Thresholds: []config.ThresholdConfig{
			{
				Metric:   "my_custom_metric",
				Stat:     "p95",
				Operator: "<",
				Target:   "200ms",
			},
		},
	}
	store := metric.NewStore()
	// Pre-register but don't record any data
	_ = store.Register("my_custom_metric", metric.MetricTypeDuration)

	diags := runner.EvaluateStrictDiagnostics(cfg, nil, store)
	assert.Len(t, diags, 1)
	assert.Equal(t, "unmatched_threshold", diags[0].Kind)
	assert.Contains(t, diags[0].Message, "my_custom_metric")
}

func TestEvaluateStrictDiagnostics_MatchedThreshold_NoDiagnostic(t *testing.T) {
	cfg := config.ScenarioConfig{
		Thresholds: []config.ThresholdConfig{
			{
				Metric:   "my_metric",
				Stat:     "count",
				Operator: ">",
				Target:   "0",
			},
		},
	}
	store := metric.NewStore()
	ctr := store.Counter("my_metric", nil)
	ctr.Inc()

	diags := runner.EvaluateStrictDiagnostics(cfg, nil, store)
	assert.Empty(t, diags)
}

func TestEvaluateStrictDiagnostics_NoUnusedParams_NoDiagnostics(t *testing.T) {
	cfg := config.ScenarioConfig{}
	store := metric.NewStore()
	diags := runner.EvaluateStrictDiagnostics(cfg, nil, store)
	assert.Empty(t, diags)
}
