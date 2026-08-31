package vuhive

import (
	"io"
	"os"
)

var osExit = defaultOSExit

func defaultOSExit(code int) {
	os.Exit(code)
}

func resetDefaultOSExit() func(int) {
	return defaultOSExit
}

// ScenarioOption configures optional lifecycle hooks when using vuhive.Run or vuhive.RunWithArgs.
type ScenarioOption func(s *Scenario)

// WithSetup configures an optional Setup hook on the scenario.
func WithSetup(fn SetupHook) ScenarioOption {
	return func(s *Scenario) {
		if s != nil {
			s.Setup = fn
		}
	}
}

// WithPreTest configures an optional per-VU PreTest hook.
func WithPreTest(fn PreTestHook) ScenarioOption {
	return func(s *Scenario) {
		if s != nil {
			s.PreTest = fn
		}
	}
}

// WithAfterTest configures an optional per-VU AfterTest hook.
func WithAfterTest(fn AfterTestHook) ScenarioOption {
	return func(s *Scenario) {
		if s != nil {
			s.AfterTest = fn
		}
	}
}

// WithTeardown configures an optional Teardown hook.
func WithTeardown(fn TeardownHook) ScenarioOption {
	return func(s *Scenario) {
		if s != nil {
			s.Teardown = fn
		}
	}
}

// WithSummary configures an optional HandleSummary hook.
func WithSummary(fn SummaryHook) ScenarioOption {
	return func(s *Scenario) {
		if s != nil {
			s.HandleSummary = fn
		}
	}
}

// Run executes a single-scenario load test suite using os.Args and terminates the process with os.Exit.
// It creates a Suite named after the scenario, registers the scenario with the given RunVU hook
// and optional lifecycle hooks, and executes the suite.
func Run(scenarioName string, runVU VURunnerHook, opts ...ScenarioOption) {
	res := RunWithArgs(scenarioName, runVU, os.Args[1:], os.Stdout, opts...)
	osExit(res.ExitCode())
}

// RunWithArgs executes a single-scenario load test suite with custom arguments and output writer.
// Unlike Run, it returns ExecutionResult and does NOT call os.Exit.
func RunWithArgs(scenarioName string, runVU VURunnerHook, args []string, stdout io.Writer, opts ...ScenarioOption) ExecutionResult {
	suite := NewSuite(scenarioName)
	sc := Scenario{
		RunVU: runVU,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&sc)
		}
	}
	suite.RegisterScenario(scenarioName, sc)
	return suite.ExecuteWithArgs(args, stdout)
}
