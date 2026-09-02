package report

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/morphy76/vuhive/internal/metric"
)

// GenerateConsoleReport formats and writes the console summary report to w.
func GenerateConsoleReport(w io.Writer, data ReportData) error {
	var sb strings.Builder

	writeHeader(&sb, data)
	writeBuiltInMetrics(&sb, data)
	writeChecks(&sb, data)
	writeGroups(&sb, data)
	writeCustomMetrics(&sb, data)
	writeStrictDiagnostics(&sb, data)
	writeSLAResults(&sb, data)
	writeVerdict(&sb, data)

	_, err := w.Write([]byte(sb.String()))
	return err
}


func writeHeader(sb *strings.Builder, data ReportData) {
	sb.WriteString("================================================================================\n")
	sb.WriteString("                        VUHIVE LOAD TEST SUMMARY\n")
	sb.WriteString("================================================================================\n")

	fmt.Fprintf(sb, "Scenario:     %-30s  Version: %s\n", data.Scenario, data.Version)

	var modeStr string
	switch data.Config.Type {
	case config.ScenarioTypeConstantVUs:
		modeStr = fmt.Sprintf("%s (%d VUs)", data.Config.Type, data.Config.VUs)
	case config.ScenarioTypeArrivalRate:
		modeStr = fmt.Sprintf("%s (%d TPS, max %d VUs)", data.Config.Type, data.Config.TargetTPS, data.Config.MaxVUs)
	default:
		modeStr = string(data.Config.Type)
	}
	fmt.Fprintf(sb, "Mode:         %-30s  Commit:  %s\n", modeStr, data.Commit)

	duration := data.EndedAt.Sub(data.StartedAt)
	durStr := fmt.Sprintf("%02d:%02d:%02d", int(duration.Hours()), int(duration.Minutes())%60, int(duration.Seconds())%60)
	if data.Config.Drain > 0 {
		fmt.Fprintf(sb, "Duration:     %s  (ramp-up: %v | run: %v | ramp-down: %v | drain: %v)\n",
			durStr, data.Config.RampUp, data.Config.RunPeriod, data.Config.RampDown, data.Config.Drain)
	} else {
		fmt.Fprintf(sb, "Duration:     %s  (ramp-up: %v | run: %v | ramp-down: %v)\n",
			durStr, data.Config.RampUp, data.Config.RunPeriod, data.Config.RampDown)
	}


	var totalIters, failedIters, timeoutIters int64
	if data.Metrics != nil {
		totalIters = data.Metrics.AggregatedCounterValue(metric.MetricIterationsTotal)
		failedIters = data.Metrics.AggregatedCounterValue(metric.MetricIterationsFailed)
		timeoutIters = data.Metrics.AggregatedCounterValue(metric.MetricIterationsTimeout)
	}

	var failPct float64
	if totalIters > 0 {
		failPct = (float64(failedIters) / float64(totalIters)) * 100
	}

	fmt.Fprintf(sb, "Iterations:   %d total  |  %d failed (%.2f%%)  |  %d timeout\n\n",
		totalIters, failedIters, failPct, timeoutIters)
}

func writeBuiltInMetrics(sb *strings.Builder, data ReportData) {
	sb.WriteString("BUILT-IN METRICS\n")
	sb.WriteString("────────────────────────────────────────────────────────────────\n")
	builtInNames := []string{
		metric.MetricIterationsTotal,
		metric.MetricIterationsFailed,
		metric.MetricIterationsTimeout,
		metric.MetricVUPanics,
		metric.MetricVUPretestErrors,
		metric.MetricChecksPassed,
		metric.MetricChecksFailed,
	}
	for _, name := range builtInNames {
		var val int64
		if data.Metrics != nil {
			val = data.Metrics.AggregatedCounterValue(name)
		}
		fmt.Fprintf(sb, "%-30s Counter    %d\n", name, val)
	}
	sb.WriteString("\n")
}

func writeChecks(sb *strings.Builder, data ReportData) {
	if data.Metrics == nil {
		return
	}
	checks := data.Metrics.CheckSummaries()
	if len(checks) == 0 {
		return
	}

	sb.WriteString("CHECKS\n")
	sb.WriteString("────────────────────────────────────────────────────────────────\n")
	fmt.Fprintf(sb, "%-30s %-10s %-8s %-8s\n", "Check Name", "Passed", "Failed", "Pass %")
	for _, ch := range checks {
		fmt.Fprintf(sb, "%-30s %-10d %-8d %.2f%%\n", ch.Name, ch.Passed, ch.Failed, ch.PassPct)
	}
	sb.WriteString("\n")
}

func writeGroups(sb *strings.Builder, data ReportData) {
	if data.Metrics == nil {
		return
	}
	groups := data.Metrics.GroupSummaries()
	if len(groups) == 0 {
		return
	}

	sb.WriteString("GROUPS\n")
	sb.WriteString("────────────────────────────────────────────────────────────────\n")
	fmt.Fprintf(sb, "%-30s %-10s %-8s %-7s %-7s %-7s %-7s %-7s\n",
		"Group Name", "Type", "Count", "Min", "Mean", "p95", "p99", "Max")
	for _, grp := range groups {
		fmt.Fprintf(sb, "%-30s %-10s %-8d %-7v %-7v %-7v %-7v %-7v\n",
			grp.Name, "Duration", grp.Count, grp.Min, grp.Mean, grp.P95, grp.P99, grp.Max)
	}
	sb.WriteString("\n")
}


type customMetricEntry struct {
	name string
	kind string
}

func writeCustomMetrics(sb *strings.Builder, data ReportData) {
	sb.WriteString("CUSTOM METRICS\n")
	sb.WriteString("────────────────────────────────────────────────────────────────\n")
	fmt.Fprintf(sb, "%-30s %-10s %-8s %-7s %-7s %-7s %-7s %-7s\n",
		"Metric", "Type", "Count", "Min", "Mean", "p95", "p99", "Max")

	if data.Metrics == nil {
		sb.WriteString("\n")
		return
	}

	customHistograms := data.Metrics.HistogramNames()
	customCounters := data.Metrics.CounterNames()
	customGauges := data.Metrics.GaugeNames()
	customRates := data.Metrics.RateNames()

	var customEntries []customMetricEntry
	for _, name := range customHistograms {
		if !strings.HasPrefix(name, metric.MetricPrefix) {
			customEntries = append(customEntries, customMetricEntry{name: name, kind: "Duration"})
		}
	}
	for _, name := range customCounters {
		if !strings.HasPrefix(name, metric.MetricPrefix) {
			customEntries = append(customEntries, customMetricEntry{name: name, kind: "Counter"})
		}
	}
	for _, name := range customGauges {
		if !strings.HasPrefix(name, metric.MetricPrefix) {
			customEntries = append(customEntries, customMetricEntry{name: name, kind: "Gauge"})
		}
	}
	for _, name := range customRates {
		if !strings.HasPrefix(name, metric.MetricPrefix) {
			customEntries = append(customEntries, customMetricEntry{name: name, kind: "Rate"})
		}
	}

	sort.Slice(customEntries, func(i, j int) bool {
		return customEntries[i].name < customEntries[j].name
	})

	for _, entry := range customEntries {
		switch entry.kind {
		case "Duration":
			snap := data.Metrics.MergedHistogramSnapshot(entry.name)
			fmt.Fprintf(sb, "%-30s %-10s %-8d %-7v %-7v %-7v %-7v %-7v\n",
				entry.name, "Duration", snap.Count, snap.Min, snap.Mean, snap.P95, snap.P99, snap.Max)
		case "Counter":
			val := data.Metrics.AggregatedCounterValue(entry.name)
			fmt.Fprintf(sb, "%-30s %-10s %-8d\n", entry.name, "Counter", val)
		case "Gauge":
			val := data.Metrics.LastGaugeValue(entry.name)
			fmt.Fprintf(sb, "%-30s %-10s (value: %g)\n", entry.name, "Gauge", val)
		case "Rate":
			val := data.Metrics.AggregatedRateValue(entry.name)
			fmt.Fprintf(sb, "%-30s %-10s (rate: %g)\n", entry.name, "Rate", val)
		}
	}
	sb.WriteString("\n")
}

func writeSLAResults(sb *strings.Builder, data ReportData) {
	sb.WriteString("SLA THRESHOLD EVALUATION\n")
	sb.WriteString("────────────────────────────────────────────────────────────────\n")
	for _, th := range data.Thresholds {
		status := "[PASS]"
		if !th.Passed {
			status = "[FAIL]"
		}
		expr := fmt.Sprintf("%s %s %s", th.Stat, th.Operator, th.Target)
		fmt.Fprintf(sb, "  %-6s  %-23s %-15s → actual: %s\n", status, th.Metric, expr, th.Actual)
	}
	sb.WriteString("────────────────────────────────────────────────────────────────\n")
}

func writeStrictDiagnostics(sb *strings.Builder, data ReportData) {
	if len(data.StrictDiagnostics) == 0 {
		return
	}
	sb.WriteString("STRICT VALIDATION DIAGNOSTICS\n")
	sb.WriteString("────────────────────────────────────────────────────────────────\n")
	for _, d := range data.StrictDiagnostics {
		fmt.Fprintf(sb, "  [STRICT WARNING]  %-18s %s\n", d.Kind, d.Message)
	}
	sb.WriteString("\n")
}

func writeVerdict(sb *strings.Builder, data ReportData) {
	verdict := "PASSED"
	exitCode := 0
	if data.Aborted {
		verdict = "ABORTED"
		exitCode = 1
		if data.AbortReason != "" {
			fmt.Fprintf(sb, "ABORT REASON: %s\n", data.AbortReason)
		}
	} else if !data.Passed {
		verdict = "FAILED"
		exitCode = 1
	}
	fmt.Fprintf(sb, "OVERALL: %-46s (exit %d)\n", verdict, exitCode)
	sb.WriteString("================================================================================\n")
}

