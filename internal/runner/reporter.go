package runner

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/morphy76/vuhive/internal/cli"
	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/engine"
	"github.com/morphy76/vuhive/internal/log"
	"github.com/morphy76/vuhive/internal/metric"
	"github.com/morphy76/vuhive/internal/report"
	"github.com/morphy76/vuhive/internal/sla"
	"github.com/morphy76/vuhive/internal/version"
	"github.com/rs/zerolog"
)

// ReportParams holds all parameters required to generate reports and invoke summary hooks.
type ReportParams struct {
	SuiteName        string
	ScenarioName     string
	Scenario         engine.Scenario
	ScenarioCfg      config.ScenarioConfig
	Flags            *cli.Flags
	MetricsStore     metric.Reader
	ThresholdResults  []sla.ThresholdResult
	StrictDiagnostics []StrictDiagnostic
	AllPassed         bool
	Aborted          bool
	AbortReason      string
	StartedAt        time.Time
	EndedAt          time.Time
	Stdout           io.Writer
	Logger           zerolog.Logger
}

// ReportExecution formats and writes test reports and triggers the HandleSummary hook.
func ReportExecution(ctx context.Context, p ReportParams) {
	reportData := report.ReportData{
		SuiteName:         p.SuiteName,
		Scenario:          p.ScenarioName,
		Version:           version.Version,
		Commit:            version.Commit,
		StartedAt:         p.StartedAt,
		EndedAt:           p.EndedAt,
		Config:            p.ScenarioCfg,
		Metrics:           p.MetricsStore,
		Thresholds:        p.ThresholdResults,
		StrictDiagnostics: convertStrictDiagnostics(p.StrictDiagnostics),
		Passed:            p.AllPassed,
		Aborted:           p.Aborted,
		AbortReason:       p.AbortReason,
	}

	reportFormat := "console"
	reportOut := ""
	jsonReportOut := ""
	if p.Flags != nil {
		if p.Flags.ReportFormat != "" {
			reportFormat = p.Flags.ReportFormat
		}
		reportOut = p.Flags.ReportOut
		jsonReportOut = p.Flags.JSONReportOut
	}

	targetWriter := p.Stdout
	if reportOut != "" {
		f, err := os.Create(reportOut)
		if err != nil {
			p.Logger.Error().Err(err).Str("path", reportOut).Msg("failed to create report output file")
			targetWriter = nil
		} else {
			defer func() {
				_ = f.Close()
			}()
			targetWriter = f
		}
	}

	if targetWriter != nil {
		if err := report.WriteReport(targetWriter, reportFormat, reportData); err != nil {
			p.Logger.Error().Err(err).Msg("failed to write report")
		}
	}

	if jsonReportOut != "" {
		f, err := os.Create(jsonReportOut)
		if err != nil {
			p.Logger.Error().Err(err).Str("path", jsonReportOut).Msg("failed to create JSON report output file")
		} else {
			defer func() {
				_ = f.Close()
			}()
			if err := report.WriteReport(f, "json", reportData); err != nil {
				p.Logger.Error().Err(err).Msg("failed to write JSON report")
			}
		}
	}


	if p.Scenario.HandleSummary != nil {
		summaryData := BuildSummaryData(SummaryParams{
			SuiteName:        p.SuiteName,
			ScenarioName:     p.ScenarioName,
			Version:          version.Version,
			Commit:           version.Commit,
			StartedAt:        p.StartedAt,
			EndedAt:          p.EndedAt,
			Config:           p.ScenarioCfg,
			MetricsStore:     p.MetricsStore,
			ThresholdResults: p.ThresholdResults,
			AllPassed:        p.AllPassed,
			Aborted:          p.Aborted,
			AbortReason:      p.AbortReason,
		})
		summaryCtx := engine.NewScenarioContext(ctx, 0, 0, p.ScenarioCfg, p.ScenarioName, nil, log.NewWithZerolog(p.Logger), nil)
		if err := p.Scenario.HandleSummary(summaryCtx, summaryData); err != nil {
			p.Logger.Error().Err(err).Msg("HandleSummary hook error")
		}
	}
}

// reportExecution is a package-private alias for ReportExecution.
func reportExecution(ctx context.Context, p ReportParams) {
	ReportExecution(ctx, p)
}

func convertStrictDiagnostics(diagnostics []StrictDiagnostic) []report.StrictDiagnosticEntry {
	if len(diagnostics) == 0 {
		return nil
	}
	entries := make([]report.StrictDiagnosticEntry, len(diagnostics))
	for i, d := range diagnostics {
		entries[i] = report.StrictDiagnosticEntry(d)
	}
	return entries
}
