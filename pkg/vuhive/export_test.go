package vuhive

import "github.com/morphy76/vuhive/internal/runner"

// Export for internal tests in vuhive_test.
func SuiteAdapterForTest(s *Suite) runner.ScenarioRegistry {
	return &runnerSuiteAdapter{suite: s}
}

// SetOSExitForTest overrides the osExit hook for testing.
func SetOSExitForTest(fn func(int)) {
	osExit = fn
}

// ResetOSExitForTest resets the osExit hook to os.Exit.
func ResetOSExitForTest() {
	osExit = resetDefaultOSExit()
}
