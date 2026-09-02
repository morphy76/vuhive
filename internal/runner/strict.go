package runner

import (
	"fmt"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/metric"
)

// StrictDiagnostic represents a single strict validation diagnostic finding.
type StrictDiagnostic struct {
	// Kind categorizes the diagnostic: "unused_param" or "unmatched_threshold".
	Kind string
	// Message is the human-readable diagnostic description.
	Message string
}

// EvaluateStrictDiagnostics detects configuration misalignments between YAML
// and runtime execution, returning diagnostics for unused params and unmatched thresholds.
func EvaluateStrictDiagnostics(
	cfg config.ScenarioConfig,
	unusedParams []string,
	metricsStore metric.Reader,
) []StrictDiagnostic {
	var diagnostics []StrictDiagnostic

	// Detect unused YAML params
	for _, key := range unusedParams {
		diagnostics = append(diagnostics, StrictDiagnostic{
			Kind:    "unused_param",
			Message: fmt.Sprintf("declared param %q was never accessed during scenario execution", key),
		})
	}

	// Detect unmatched threshold metrics (registered but never observed)
	if metricsStore != nil {
		for _, th := range cfg.Thresholds {
			if th.Metric == "" {
				continue
			}
			if !hasObservedData(th, metricsStore) {
				diagnostics = append(diagnostics, StrictDiagnostic{
					Kind:    "unmatched_threshold",
					Message: fmt.Sprintf("threshold metric %q (stat: %s) was never recorded during scenario execution", th.Metric, th.Stat),
				})
			}
		}
	}

	return diagnostics
}

// hasObservedData checks whether a threshold's target metric received any actual observations.
func hasObservedData(th config.ThresholdConfig, reader metric.Reader) bool {
	_, exists := reader.MetricType(th.Metric)
	if !exists {
		return false
	}

	if config.IsDurationStat(th.Stat) {
		snap := reader.MergedHistogramSnapshot(th.Metric)
		return snap.Count > 0
	}

	switch th.Stat {
	case "count":
		return reader.AggregatedCounterValue(th.Metric) > 0
	case "rate":
		_, hasData := reader.RateData(th.Metric)
		return hasData
	case "value":
		// For gauges, a registered metric with value 0 could be valid data.
		// Since we pre-register threshold metrics, existence alone isn't sufficient.
		// We check if any gauge was actually Set() by checking if the metric was
		// registered via scenario code (not pre-registration). Since pre-registration
		// only registers the type in the registry, and actual gauge instances are
		// created by collector.Gauge(), we can check if the aggregator returns a non-zero value.
		// However, 0 could be a valid gauge value, so we report it as unmatched if zero.
		// This is a best-effort heuristic — a real gauge set to exactly 0 would be a false positive.
		return reader.LastGaugeValue(th.Metric) != 0
	}

	return false
}

// ResolveStrictMode determines the effective strict mode from CLI flags and YAML config.
// Priority: --strict-fatal > --strict > YAML strict setting.
func ResolveStrictMode(cliStrict, cliStrictFatal bool, yamlStrict string) string {
	if cliStrictFatal {
		return config.StrictModeFatal
	}
	if cliStrict {
		return config.StrictModeWarn
	}
	if yamlStrict != "" && yamlStrict != config.StrictModeOff {
		return yamlStrict
	}
	return ""
}
