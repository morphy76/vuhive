package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func findSchemaPath(t *testing.T) string {
	// Locate schemas/vuhive.schema.json relative to repository root
	candidates := []string{
		"../../schemas/vuhive.schema.json",
		"../schemas/vuhive.schema.json",
		"schemas/vuhive.schema.json",
	}
	for _, c := range candidates {
		if abs, err := filepath.Abs(c); err == nil {
			if _, err := os.Stat(abs); err == nil {
				return abs
			}
		}
	}
	t.Fatalf("schemas/vuhive.schema.json not found in candidate paths")
	return ""
}

func loadSchema(t *testing.T) map[string]any {
	schemaPath := findSchemaPath(t)
	data, err := os.ReadFile(schemaPath)
	require.NoError(t, err)

	var schema map[string]any
	err = json.Unmarshal(data, &schema)
	require.NoError(t, err, "schemas/vuhive.schema.json must be valid JSON")
	return schema
}

func TestJSONSchema_StructuralValidity(t *testing.T) {
	schema := loadSchema(t)

	// Validate top-level schema metadata
	assert.NotEmpty(t, schema["$schema"])
	assert.NotEmpty(t, schema["$id"])
	assert.Equal(t, "vuhive Configuration Schema", schema["title"])
	assert.NotEmpty(t, schema["description"])

	// Validate required top-level properties
	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, props, "version")
	assert.Contains(t, props, "default_scenario")
	assert.Contains(t, props, "scenarios")

	// Validate required array
	required, ok := schema["required"].([]any)
	require.True(t, ok)
	assert.Contains(t, required, "version")
	assert.Contains(t, required, "scenarios")

	// Validate $defs / definitions exist
	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		defs, ok = schema["definitions"].(map[string]any)
	}
	require.True(t, ok, "schema must define $defs or definitions")
	assert.Contains(t, defs, "scenario")
	assert.Contains(t, defs, "stage")
	assert.Contains(t, defs, "think_time")
	assert.Contains(t, defs, "threshold")
	assert.Contains(t, defs, "duration")

	scenarioDef, ok := defs["scenario"].(map[string]any)
	require.True(t, ok)
	scenarioProps, ok := scenarioDef["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, scenarioProps, "drain")
	assert.Contains(t, scenarioProps, "drain_period")
}


func TestJSONSchema_DurationRegex(t *testing.T) {
	schema := loadSchema(t)

	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		defs = schema["definitions"].(map[string]any)
	}
	require.NotNil(t, defs)

	durationDef, ok := defs["duration"].(map[string]any)
	require.True(t, ok)
	patternStr, ok := durationDef["pattern"].(string)
	require.True(t, ok)

	re, err := regexp.Compile(patternStr)
	require.NoError(t, err)

	validDurations := []string{
		"0s",
		"500ms",
		"10s",
		"1m",
		"2m30s",
		"1h",
		"100us",
		"50µs",
		"1000ns",
		"1.5s",
		"2.5m",
	}

	for _, d := range validDurations {
		assert.True(t, re.MatchString(d), "expected valid duration %q to match pattern %q", d, patternStr)
	}

	invalidDurations := []string{
		"",
		"10",
		"seconds",
		"10sec",
		"-5s",
		"10 s",
		"abc",
	}

	for _, d := range invalidDurations {
		assert.False(t, re.MatchString(d), "expected invalid duration %q to fail match against pattern %q", d, patternStr)
	}
}

func TestJSONSchema_ScenarioTypeValidation(t *testing.T) {
	schema := loadSchema(t)

	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		defs = schema["definitions"].(map[string]any)
	}
	scenarioDef, ok := defs["scenario"].(map[string]any)
	require.True(t, ok)

	scenarioProps, ok := scenarioDef["properties"].(map[string]any)
	require.True(t, ok)

	typeProp, ok := scenarioProps["type"].(map[string]any)
	require.True(t, ok)

	enumList, ok := typeProp["enum"].([]any)
	require.True(t, ok)

	assert.Contains(t, enumList, "constant_vus")
	assert.Contains(t, enumList, "arrival_rate")
	assert.Contains(t, enumList, "ramping_vus")
}

func TestJSONSchema_ThresholdStatsAndOperators(t *testing.T) {
	schema := loadSchema(t)

	defs := schema["$defs"].(map[string]any)
	thresholdDef := defs["threshold"].(map[string]any)
	thresholdProps := thresholdDef["properties"].(map[string]any)

	statProp := thresholdProps["stat"].(map[string]any)
	statEnums := statProp["enum"].([]any)
	expectedStats := []string{"p50", "p90", "p95", "p99", "mean", "max", "count", "rate", "value"}
	for _, s := range expectedStats {
		assert.Contains(t, statEnums, s)
	}

	operatorProp := thresholdProps["operator"].(map[string]any)
	opEnums := operatorProp["enum"].([]any)
	expectedOps := []string{"<", "<=", ">", ">="}
	for _, op := range expectedOps {
		assert.Contains(t, opEnums, op)
	}

	onNoDataProp, ok := thresholdProps["on_no_data"].(map[string]any)
	require.True(t, ok, "threshold should have on_no_data property")
	onNoDataEnums := onNoDataProp["enum"].([]any)
	expectedStrategies := []string{"zero", "fail", "pass", "ignore", "skip"}
	for _, strat := range expectedStrategies {
		assert.Contains(t, onNoDataEnums, strat)
	}
}

func TestJSONSchema_ThinkTimeStrategies(t *testing.T) {
	schema := loadSchema(t)

	defs := schema["$defs"].(map[string]any)
	thinkTimeDef := defs["think_time"].(map[string]any)
	thinkTimeProps := thinkTimeDef["properties"].(map[string]any)

	typeProp := thinkTimeProps["type"].(map[string]any)
	typeEnums := typeProp["enum"].([]any)
	expectedTypes := []string{"fixed", "range", "expo", "gaussian"}
	for _, tt := range expectedTypes {
		assert.Contains(t, typeEnums, tt)
	}
}

func TestJSONSchema_YAMLValidationSamples(t *testing.T) {
	validConstantVUs := `
version: "1.0"
default_scenario: test_constant
scenarios:
  test_constant:
    type: constant_vus
    vus: 10
    ramp_up: 5s
    run_period: 30s
    ramp_down: 5s
    vu_timeout: 2s
    interaction_delay:
      type: range
      min: 100ms
      max: 500ms
    thresholds:
      - metric: http_req_duration
        stat: p95
        operator: "<"
        target: "200ms"
        abort_on_fail: true
        delay_abort_eval: 5s
`

	validArrivalRate := `
version: "1.0"
scenarios:
  test_arrival:
    type: arrival_rate
    target_tps: 100
    max_vus: 100
    run_period: 1m
    vu_timeout: 1s
    thresholds:
      - metric: vuhive.pacing.dropped_iterations
        stat: count
        operator: "<="
        target: "0"
`

	validRampingVUs := `
version: "1.0"
scenarios:
  test_ramping:
    type: ramping_vus
    stages:
      - target: 10
        duration: 30s
      - target: 50
        duration: 10s
      - target: 0
        duration: 30s
    vu_timeout: 2s
`

	for name, rawYAML := range map[string]string{
		"constant_vus": validConstantVUs,
		"arrival_rate": validArrivalRate,
		"ramping_vus":  validRampingVUs,
	} {
		t.Run(name, func(t *testing.T) {
			var parsed map[string]any
			err := yaml.Unmarshal([]byte(rawYAML), &parsed)
			require.NoError(t, err)
			assert.Equal(t, "1.0", parsed["version"])
			scenarios, ok := parsed["scenarios"].(map[string]any)
			require.True(t, ok)
			assert.NotEmpty(t, scenarios)
		})
	}
}

func TestJSONSchema_HTTPConfig(t *testing.T) {
	schema := loadSchema(t)

	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		defs, ok = schema["definitions"].(map[string]any)
	}
	require.True(t, ok)
	assert.Contains(t, defs, "http_config")

	scenarioDef, ok := defs["scenario"].(map[string]any)
	require.True(t, ok)
	scenarioProps, ok := scenarioDef["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, scenarioProps, "http")

	httpDef, ok := defs["http_config"].(map[string]any)
	require.True(t, ok)
	httpProps, ok := httpDef["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, httpProps, "base_url")
	assert.Contains(t, httpProps, "timeout")
	assert.Contains(t, httpProps, "headers")
	assert.Contains(t, httpProps, "tls")
	assert.Contains(t, httpProps, "pool")
	assert.Contains(t, httpProps, "detailed_timing")
	assert.Contains(t, httpProps, "metric_prefix")

	poolProp, ok := httpProps["pool"].(map[string]any)
	require.True(t, ok)
	poolProps, ok := poolProp["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, poolProps, "max_idle_conns")
	assert.Contains(t, poolProps, "max_idle_conns_per_host")
	assert.Contains(t, poolProps, "idle_conn_timeout")

	tlsProp, ok := httpProps["tls"].(map[string]any)
	require.True(t, ok)
	tlsProps, ok := tlsProp["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, tlsProps, "insecure_skip_verify")
}



func TestJSONSchema_HTTPClients(t *testing.T) {
	schema := loadSchema(t)

	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		defs, ok = schema["definitions"].(map[string]any)
	}
	require.True(t, ok)

	scenarioDef, ok := defs["scenario"].(map[string]any)
	require.True(t, ok)
	scenarioProps, ok := scenarioDef["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, scenarioProps, "http_clients")

	httpClientsDef, ok := scenarioProps["http_clients"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "object", httpClientsDef["type"])
	
	additionalProps, ok := httpClientsDef["additionalProperties"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "#/$defs/http_config", additionalProps["$ref"])
}
