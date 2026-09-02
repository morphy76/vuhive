package report

import (
	"time"
)

// CheckSummary represents aggregated results for a named inline check.
type CheckSummary struct {
	Name    string
	Passed  int64
	Failed  int64
	Total   int64
	PassPct float64
}

// MetricSummary represents a metric entry in the execution summary.
type MetricSummary struct {
	Name   string
	Type   string
	Tags   map[string]string
	Count  int64
	Value  float64
	Rate   float64
	Min    time.Duration
	Mean   time.Duration
	P50    time.Duration
	P90    time.Duration
	P95    time.Duration
	P99    time.Duration
	Max    time.Duration
}

// ThresholdSummary represents the outcome of a single SLA threshold evaluation.
type ThresholdSummary struct {
	Metric   string
	Stat     string
	Operator string
	Target   string
	Actual   string
	Passed   bool
}

// GroupSummary represents aggregated latency statistics for a named transaction group.
type GroupSummary struct {
	Name  string
	Count int64
	Min   time.Duration
	Mean  time.Duration
	P50   time.Duration
	P90   time.Duration
	P95   time.Duration
	P99   time.Duration
	Max   time.Duration
}

// SummaryData contains the complete structured report information post-execution.
type SummaryData struct {
	SuiteName   string
	Scenario    string
	Version     string
	Commit      string
	StartedAt   time.Time
	EndedAt     time.Time
	Duration    time.Duration
	Config      any
	Metrics     []MetricSummary
	Checks      []CheckSummary
	Groups      []GroupSummary
	Thresholds  []ThresholdSummary
	Passed            bool
	Aborted           bool
	AbortReason       string
	StrictDiagnostics []StrictDiagnosticEntry
}


// Metric returns the MetricSummary for the given metric name, or nil if not found.
func (s SummaryData) Metric(name string) *MetricSummary {
	for i := range s.Metrics {
		if s.Metrics[i].Name == name {
			return &s.Metrics[i]
		}
	}
	return nil
}

// Counter returns the count value for the named counter metric, or 0 if not found.
func (s SummaryData) Counter(name string) int64 {
	m := s.Metric(name)
	if m == nil {
		return 0
	}
	return m.Count
}

// Rate returns the rate value for the named rate metric, or 0 if not found.
func (s SummaryData) Rate(name string) float64 {
	m := s.Metric(name)
	if m == nil {
		return 0
	}
	return m.Rate
}

// Gauge returns the value for the named gauge metric, or 0 if not found.
func (s SummaryData) Gauge(name string) float64 {
	m := s.Metric(name)
	if m == nil {
		return 0
	}
	return m.Value
}

// Threshold returns the ThresholdSummary for the given metric name, or nil if not found.
func (s SummaryData) Threshold(metric string) *ThresholdSummary {
	for i := range s.Thresholds {
		if s.Thresholds[i].Metric == metric {
			return &s.Thresholds[i]
		}
	}
	return nil
}

// Group returns the GroupSummary for the given group name, or nil if not found.
func (s SummaryData) Group(name string) *GroupSummary {
	for i := range s.Groups {
		if s.Groups[i].Name == name {
			return &s.Groups[i]
		}
	}
	return nil
}

