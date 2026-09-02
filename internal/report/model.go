package report

import (
	"io"
	"strings"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/metric"
	"github.com/morphy76/vuhive/internal/sla"
)

// ReportData gathers all scenario metadata, metric store snapshots, and SLA results for reporting.
type ReportData struct {
	SuiteName   string
	Scenario    string
	Version     string
	Commit      string
	StartedAt   time.Time
	EndedAt     time.Time
	Config      config.ScenarioConfig
	Metrics     metric.Reader
	Thresholds  []sla.ThresholdResult
	Passed            bool
	Aborted           bool
	AbortReason       string
	StrictDiagnostics []StrictDiagnosticEntry
}

// StrictDiagnosticEntry represents a strict validation diagnostic for report output.
type StrictDiagnosticEntry struct {
	Kind    string
	Message string
}

// WriteReport outputs the report in the requested format (console or json) to w.
func WriteReport(w io.Writer, format string, data ReportData) error {
	switch strings.ToLower(format) {
	case "json":
		return GenerateJSONReport(w, data)
	default:
		return GenerateConsoleReport(w, data)
	}
}

