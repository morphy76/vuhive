package runner

import (
	"cmp"
	"slices"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/metric"
	"github.com/morphy76/vuhive/internal/report"
	"github.com/morphy76/vuhive/internal/sla"
)

// SummaryParams encapsulates parameters required to build execution summary data.
type SummaryParams struct {
	SuiteName        string
	ScenarioName     string
	Version          string
	Commit           string
	StartedAt        time.Time
	EndedAt          time.Time
	Config           config.ScenarioConfig
	MetricsStore     metric.Reader
	ThresholdResults  []sla.ThresholdResult
	StrictDiagnostics []report.StrictDiagnosticEntry
	AllPassed         bool
	Aborted           bool
	AbortReason       string
}

// BuildSummaryData constructs report.SummaryData from SummaryParams using standard library sorting.
func BuildSummaryData(p SummaryParams) report.SummaryData {
	duration := p.EndedAt.Sub(p.StartedAt)
	if duration < 0 {
		duration = 0
	}

	thresholds := make([]report.ThresholdSummary, 0, len(p.ThresholdResults))
	for _, th := range p.ThresholdResults {
		thresholds = append(thresholds, report.ThresholdSummary{
			Metric:   th.Metric,
			Stat:     th.Stat,
			Operator: th.Operator,
			Target:   th.Target,
			Actual:   th.Actual,
			Passed:   th.Passed,
		})
	}

	type namedEntry struct {
		name string
		item report.MetricSummary
	}
	var entries []namedEntry

	if p.MetricsStore != nil {
		// Histograms
		for _, name := range p.MetricsStore.HistogramNames() {
			snap := p.MetricsStore.MergedHistogramSnapshot(name)
			entries = append(entries, namedEntry{
				name: name,
				item: report.MetricSummary{
					Name:  name,
					Type:  "duration",
					Count: snap.Count,
					Min:   snap.Min,
					Mean:  snap.Mean,
					P50:   snap.P50,
					P90:   snap.P90,
					P95:   snap.P95,
					P99:   snap.P99,
					Max:   snap.Max,
				},
			})
		}

		// Counters
		for _, name := range p.MetricsStore.CounterNames() {
			val := p.MetricsStore.AggregatedCounterValue(name)
			entries = append(entries, namedEntry{
				name: name,
				item: report.MetricSummary{
					Name:  name,
					Type:  "counter",
					Count: val,
				},
			})
		}

		// Gauges
		for _, name := range p.MetricsStore.GaugeNames() {
			val := p.MetricsStore.LastGaugeValue(name)
			entries = append(entries, namedEntry{
				name: name,
				item: report.MetricSummary{
					Name:  name,
					Type:  "gauge",
					Value: val,
				},
			})
		}

		// Rates
		for _, name := range p.MetricsStore.RateNames() {
			val := p.MetricsStore.AggregatedRateValue(name)
			entries = append(entries, namedEntry{
				name: name,
				item: report.MetricSummary{
					Name: name,
					Type: "rate",
					Rate: val,
				},
			})
		}
	}

	// Sort metrics alphabetically by name using standard slices.SortFunc (O(n log n))
	slices.SortFunc(entries, func(a, b namedEntry) int {
		return cmp.Compare(a.name, b.name)
	})

	metrics := make([]report.MetricSummary, 0, len(entries))
	for _, e := range entries {
		metrics = append(metrics, e.item)
	}

	var checks []report.CheckSummary
	if p.MetricsStore != nil {
		for _, cs := range p.MetricsStore.CheckSummaries() {
			checks = append(checks, report.CheckSummary{
				Name:    cs.Name,
				Passed:  cs.Passed,
				Failed:  cs.Failed,
				Total:   cs.Total,
				PassPct: cs.PassPct,
			})
		}
	}

	var groups []report.GroupSummary
	if p.MetricsStore != nil {
		for _, grp := range p.MetricsStore.GroupSummaries() {
			groups = append(groups, report.GroupSummary{
				Name:  grp.Name,
				Count: grp.Count,
				Min:   grp.Min,
				Mean:  grp.Mean,
				P50:   grp.P50,
				P90:   grp.P90,
				P95:   grp.P95,
				P99:   grp.P99,
				Max:   grp.Max,
			})
		}
	}

	return report.SummaryData{
		SuiteName:   p.SuiteName,
		Scenario:    p.ScenarioName,
		Version:     p.Version,
		Commit:      p.Commit,
		StartedAt:   p.StartedAt,
		EndedAt:     p.EndedAt,
		Duration:    duration,
		Config:      p.Config,
		Metrics:     metrics,
		Checks:      checks,
		Groups:      groups,
		Thresholds:        thresholds,
		Passed:            p.AllPassed,
		Aborted:           p.Aborted,
		AbortReason:       p.AbortReason,
		StrictDiagnostics: p.StrictDiagnostics,
	}
}

