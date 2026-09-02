package vuhive_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/morphy76/vuhive/pkg/vuhive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunWithArgs_ReturnsExecutionResultWithoutExit(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "vuhive.yaml")

	yamlContent := `version: "1.0"
default_scenario: quick_test
scenarios:
  quick_test:
    type: constant_vus
    vus: 1
    run_period: 50ms
    vu_timeout: 1s
`
	require.NoError(t, os.WriteFile(configPath, []byte(yamlContent), 0644))

	var runCount atomic.Int64
	var stdout bytes.Buffer

	res := vuhive.RunWithArgs(
		"quick_test",
		func(ctx vuhive.VUContext) error {
			runCount.Add(1)
			return nil
		},
		[]string{"--config", configPath},
		&stdout,
	)

	require.NoError(t, res.Error)
	assert.True(t, res.Passed)
	assert.False(t, res.Aborted)
	assert.Equal(t, 0, res.ExitCode())
	assert.Positive(t, runCount.Load())
}

func TestRunWithArgs_WithOptions_AppliesAllHooks(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "vuhive.yaml")

	yamlContent := `version: "1.0"
default_scenario: hooked_scenario
scenarios:
  hooked_scenario:
    type: constant_vus
    vus: 1
    run_period: 50ms
    vu_timeout: 1s
`
	require.NoError(t, os.WriteFile(configPath, []byte(yamlContent), 0644))

	var (
		setupCalled    atomic.Bool
		preTestCalled  atomic.Bool
		runVUCalled    atomic.Bool
		afterTestCalls atomic.Int64
		teardownCalled atomic.Bool
		summaryCalled  atomic.Bool
	)

	var stdout bytes.Buffer
	res := vuhive.RunWithArgs(
		"hooked_scenario",
		func(ctx vuhive.VUContext) error {
			runVUCalled.Store(true)
			val := ctx.GlobalState("init_key")
			assert.Equal(t, "init_val", val)
			return nil
		},
		[]string{"--config", configPath},
		&stdout,
		vuhive.WithSetup(func(ctx vuhive.SetupContext) (map[string]any, error) {
			setupCalled.Store(true)
			return map[string]any{"init_key": "init_val"}, nil
		}),
		vuhive.WithPreTest(func(ctx vuhive.VUContext) error {
			preTestCalled.Store(true)
			return nil
		}),
		vuhive.WithAfterTest(func(ctx vuhive.VUContext) error {
			afterTestCalls.Add(1)
			return nil
		}),
		vuhive.WithTeardown(func(ctx vuhive.TeardownContext, state map[string]any) error {
			teardownCalled.Store(true)
			assert.Equal(t, "init_val", state["init_key"])
			return nil
		}),
		vuhive.WithSummary(func(ctx vuhive.SummaryContext, summary vuhive.SummaryData) error {
			summaryCalled.Store(true)
			assert.Equal(t, "hooked_scenario", summary.Scenario)
			return nil
		}),
	)

	require.NoError(t, res.Error)
	assert.True(t, res.Passed)
	assert.Equal(t, 0, res.ExitCode())
	assert.True(t, setupCalled.Load(), "Setup should be called")
	assert.True(t, preTestCalled.Load(), "PreTest should be called")
	assert.True(t, runVUCalled.Load(), "RunVU should be called")
	assert.Positive(t, afterTestCalls.Load(), "AfterTest should be called at least once")
	assert.True(t, teardownCalled.Load(), "Teardown should be called")
	assert.True(t, summaryCalled.Load(), "HandleSummary should be called")
}

func TestRun_ExecutesScenarioWithDefaultArgs(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "vuhive.yaml")

	yamlContent := `version: "1.0"
default_scenario: default_args_test
scenarios:
  default_args_test:
    type: constant_vus
    vus: 1
    run_period: 50ms
    vu_timeout: 1s
`
	require.NoError(t, os.WriteFile(configPath, []byte(yamlContent), 0644))

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"test_binary", "--config", configPath}

	var exitCode int
	var exitCalled bool
	vuhive.SetOSExitForTest(func(code int) {
		exitCode = code
		exitCalled = true
	})
	defer vuhive.ResetOSExitForTest()

	var runCount atomic.Int64
	vuhive.Run(
		"default_args_test",
		func(ctx vuhive.VUContext) error {
			runCount.Add(1)
			return nil
		},
	)

	assert.True(t, exitCalled, "Run should have called osExit")
	assert.Equal(t, 0, exitCode)
	assert.Positive(t, runCount.Load())
}

func TestRun_ExitCodeFailureOnConfigError(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"test_binary", "--config", "nonexistent.yaml"}

	var exitCode int
	var exitCalled bool
	vuhive.SetOSExitForTest(func(code int) {
		exitCode = code
		exitCalled = true
	})
	defer vuhive.ResetOSExitForTest()

	vuhive.Run(
		"nonexistent_config_test",
		func(ctx vuhive.VUContext) error {
			return nil
		},
	)

	assert.True(t, exitCalled, "Run should have called osExit")
	assert.Equal(t, 1, exitCode)
}

func TestRun_PanicsOnEmptyNameOrNilRunVU(t *testing.T) {
	t.Run("empty scenario name panics", func(t *testing.T) {
		assert.PanicsWithValue(t, "vuhive: RegisterScenario called with empty name", func() {
			vuhive.Run("", func(ctx vuhive.VUContext) error {
				return nil
			})
		})
	})

	t.Run("nil RunVU panics", func(t *testing.T) {
		assert.PanicsWithValue(t, "vuhive: RegisterScenario called with nil RunVU for scenario valid_name", func() {
			vuhive.Run("valid_name", nil)
		})
	})

	t.Run("RunWithArgs empty scenario name panics", func(t *testing.T) {
		assert.PanicsWithValue(t, "vuhive: RegisterScenario called with empty name", func() {
			vuhive.RunWithArgs("", func(ctx vuhive.VUContext) error {
				return nil
			}, nil, nil)
		})
	})

	t.Run("RunWithArgs nil RunVU panics", func(t *testing.T) {
		assert.PanicsWithValue(t, "vuhive: RegisterScenario called with nil RunVU for scenario valid_name", func() {
			vuhive.RunWithArgs("valid_name", nil, nil, nil)
		})
	})
}

func TestRunWithArgs_NilOptionIgnored(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "vuhive.yaml")

	yamlContent := `version: "1.0"
default_scenario: nil_opt_test
scenarios:
  nil_opt_test:
    type: constant_vus
    vus: 1
    run_period: 50ms
    vu_timeout: 1s
`
	require.NoError(t, os.WriteFile(configPath, []byte(yamlContent), 0644))

	var stdout bytes.Buffer
	res := vuhive.RunWithArgs(
		"nil_opt_test",
		func(ctx vuhive.VUContext) error {
			return nil
		},
		[]string{"--config", configPath},
		&stdout,
		nil,
	)

	require.NoError(t, res.Error)
	assert.True(t, res.Passed)
}

func TestRunWithArgs_StartupQuorumError(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "vuhive.yaml")

	yamlContent := `version: "1.0"
default_scenario: quorum_test
scenarios:
  quorum_test:
    type: constant_vus
    vus: 5
    run_period: 1s
    vu_timeout: 1s
    max_pretest_retries: 0
    min_ready_ratio: 0.8
    startup_grace_period: 500ms
`
	require.NoError(t, os.WriteFile(configPath, []byte(yamlContent), 0644))

	var stdout bytes.Buffer
	res := vuhive.RunWithArgs(
		"quorum_test",
		func(ctx vuhive.VUContext) error {
			return nil
		},
		[]string{"--config", configPath},
		&stdout,
		vuhive.WithPreTest(func(ctx vuhive.VUContext) error {
			if ctx.VUID() > 2 {
				return errors.New("auth failed")
			}
			return nil
		}),
	)

	require.Error(t, res.Error)
	assert.False(t, res.Passed)
	assert.Equal(t, 1, res.ExitCode())
	assert.True(t, errors.Is(res.Error, vuhive.ErrStartupQuorumFailed))

	var quorumErr *vuhive.StartupQuorumError
	require.True(t, errors.As(res.Error, &quorumErr))
	assert.Equal(t, 5, quorumErr.Target)
	assert.Equal(t, 4, quorumErr.Required)
	assert.Equal(t, 0.8, quorumErr.Ratio)
}

