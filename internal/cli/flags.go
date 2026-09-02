// Package cli provides command-line flag parsing for vuhive suite execution.
package cli

import (
	"flag"
	"fmt"
	"io"
)

// Flags holds the parsed command-line configuration options.
type Flags struct {
	// ConfigPath is the filesystem path to the YAML configuration file.
	ConfigPath string
	// ScenarioName is the name of the specific scenario to execute.
	ScenarioName string
	// LogLevel sets the logging verbosity (debug, info, warn, error).
	LogLevel string
	// LogFormat sets the log output formatting (pretty or json).
	LogFormat string
	// ReportFormat sets the output format for the summary report (console or json).
	ReportFormat string
	// ReportOut specifies an optional file path to write the console summary report to.
	ReportOut string
	// JSONReportOut specifies an optional file path to write the JSON report document to.
	JSONReportOut string
	// ShowVersion indicates whether to print the library version and exit.
	ShowVersion bool
	// Strict enables strict validation (warn about unused params and unmatched metrics).
	Strict bool
	// StrictFatal like Strict but fails execution.
	StrictFatal bool
}

// ParseFlags parses command-line arguments into a Flags struct.
// It uses a local FlagSet to allow isolated unit testing without global state mutation.
func ParseFlags(args []string, errOutput io.Writer) (*Flags, error) {
	fs := flag.NewFlagSet("vuhive", flag.ContinueOnError)
	if errOutput != nil {
		fs.SetOutput(errOutput)
	}

	flags := &Flags{}
	fs.StringVar(&flags.ConfigPath, "config", "vuhive.yaml", "Path to the YAML configuration file")
	fs.StringVar(&flags.ScenarioName, "scenario", "", "Name of the scenario to execute")
	fs.StringVar(&flags.LogLevel, "log-level", "info", "Log verbosity: debug, info, warn, error")
	fs.StringVar(&flags.LogFormat, "log-format", "pretty", "Log output format: pretty or json")
	fs.StringVar(&flags.ReportFormat, "report-format", "console", "Report format: console or json")
	fs.StringVar(&flags.ReportOut, "report-out", "", "Write final report to this file path instead of stdout")
	fs.StringVar(&flags.JSONReportOut, "json-report-out", "", "Write JSON report document to this file path")
	fs.BoolVar(&flags.ShowVersion, "version", false, "Print library version and exit")
	fs.BoolVar(&flags.Strict, "strict", false, "Enable strict validation: warn about unused YAML params and unmatched threshold metrics")
	fs.BoolVar(&flags.StrictFatal, "strict-fatal", false, "Like --strict, but exit with code 1 if diagnostics are found")

	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("vuhive: invalid command line flags: %w", err)
	}

	return flags, nil
}
