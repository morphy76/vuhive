package vuhive_test

import (
	"context"
	"testing"
	"time"

	"github.com/morphy76/vuhive/pkg/vuhive"
	"github.com/morphy76/vuhive/pkg/vuhive/data"
	"github.com/stretchr/testify/assert"
)

// Compile-time checks for interface satisfaction
var (
	_ context.Context           = (vuhive.ScenarioContext)(nil)
	_ vuhive.ExecutionIdentity   = (vuhive.ScenarioContext)(nil)
	_ vuhive.ConfigProvider      = (vuhive.ScenarioContext)(nil)
	_ vuhive.StateProvider       = (vuhive.ScenarioContext)(nil)
	_ vuhive.ObservabilityProvider = (vuhive.ScenarioContext)(nil)
	_ vuhive.WorkflowController  = (vuhive.ScenarioContext)(nil)
	_ data.ContextAccessor      = (vuhive.ExecutionIdentity)(nil)
	_ data.ContextAccessor      = (vuhive.ScenarioContext)(nil)

	// Role-specific context interfaces
	_ context.Context           = (vuhive.SetupContext)(nil)
	_ vuhive.ConfigProvider      = (vuhive.SetupContext)(nil)
	_ vuhive.ObservabilityProvider = (vuhive.SetupContext)(nil)

	_ context.Context           = (vuhive.VUContext)(nil)
	_ vuhive.ExecutionIdentity   = (vuhive.VUContext)(nil)
	_ vuhive.ConfigProvider      = (vuhive.VUContext)(nil)
	_ vuhive.StateProvider       = (vuhive.VUContext)(nil)
	_ vuhive.ObservabilityProvider = (vuhive.VUContext)(nil)
	_ vuhive.WorkflowController  = (vuhive.VUContext)(nil)

	_ context.Context           = (vuhive.TeardownContext)(nil)
	_ vuhive.ConfigProvider      = (vuhive.TeardownContext)(nil)
	_ vuhive.StateProvider       = (vuhive.TeardownContext)(nil)
	_ vuhive.ObservabilityProvider = (vuhive.TeardownContext)(nil)

	_ context.Context           = (vuhive.SummaryContext)(nil)
	_ vuhive.ConfigProvider      = (vuhive.SummaryContext)(nil)
	_ vuhive.ObservabilityProvider = (vuhive.SummaryContext)(nil)
)

// Mock implementations verifying that clients only need to implement the small interface they consume

type mockIdentity struct{}

func (mockIdentity) VUID() int64           { return 42 }
func (mockIdentity) Iteration() int64      { return 7 }
func (mockIdentity) ScenarioName() string  { return "mock_scenario" }

type mockConfig struct{}

func (mockConfig) Param(key string) string                                    { return "val_" + key }
func (mockConfig) ParamInt(key string, defaultValue int) int                  { return defaultValue + 1 }
func (mockConfig) ParamDuration(key string, defaultValue time.Duration) time.Duration {
	return defaultValue + time.Second
}
func (mockConfig) HTTPConfig() vuhive.HTTPConfig {
	return vuhive.HTTPConfig{BaseURL: "https://api.example.com"}
}
func (mockConfig) HTTPClients() map[string]vuhive.HTTPConfig {
	return map[string]vuhive.HTTPConfig{
		"api": {BaseURL: "https://api.example.com"},
	}
}

type mockState struct{}

func (mockState) GlobalState(key string) any {
	if key == "token" {
		return "secret"
	}
	return nil
}

type mockWorkflow struct {
	slept       bool
	checkResult bool
}

func (m *mockWorkflow) Sleep(d ...time.Duration) error {
	m.slept = true
	return nil
}

func (m *mockWorkflow) Check(name string, fn vuhive.CheckFunc) bool {
	if fn != nil && fn() == "" {
		m.checkResult = true
		return true
	}
	m.checkResult = false
	return false
}

func (m *mockWorkflow) CheckEqual(name string, actual, expected any) bool {
	res := actual == expected
	m.checkResult = res
	return res
}

func (m *mockWorkflow) CheckTrue(name string, condition bool, failureReason ...string) bool {
	m.checkResult = condition
	return condition
}

func (m *mockWorkflow) CheckNoError(name string, err error) bool {
	res := err == nil
	m.checkResult = res
	return res
}

func (m *mockWorkflow) Group(name string, fn func(ctx vuhive.VUContext) error) error {
	if fn != nil {
		return fn(nil)
	}
	return nil
}

func TestInterfaceSegregation_ExecutionIdentity(t *testing.T) {

	var identity vuhive.ExecutionIdentity = mockIdentity{}
	assert.Equal(t, int64(42), identity.VUID())
	assert.Equal(t, int64(7), identity.Iteration())
	assert.Equal(t, "mock_scenario", identity.ScenarioName())

	// Satisfies data.ContextAccessor
	var accessor data.ContextAccessor = identity
	assert.Equal(t, int64(42), accessor.VUID())
	assert.Equal(t, int64(7), accessor.Iteration())
}

func TestInterfaceSegregation_ConfigProvider(t *testing.T) {
	var cfg vuhive.ConfigProvider = mockConfig{}
	assert.Equal(t, "val_host", cfg.Param("host"))
	assert.Equal(t, 11, cfg.ParamInt("port", 10))
	assert.Equal(t, 2*time.Second, cfg.ParamDuration("timeout", time.Second))
	assert.Equal(t, "https://api.example.com", cfg.HTTPConfig().BaseURL)
}

func TestInterfaceSegregation_StateProvider(t *testing.T) {
	var state vuhive.StateProvider = mockState{}
	assert.Equal(t, "secret", state.GlobalState("token"))
	assert.Nil(t, state.GlobalState("unknown"))
}

func TestInterfaceSegregation_WorkflowController(t *testing.T) {
	wf := &mockWorkflow{}
	var controller vuhive.WorkflowController = wf

	err := controller.Sleep(10 * time.Millisecond)
	assert.NoError(t, err)
	assert.True(t, wf.slept)

	passed := controller.Check("test_pass", func() string { return "" })
	assert.True(t, passed)
	assert.True(t, wf.checkResult)

	failed := controller.Check("test_fail", func() string { return "error" })
	assert.False(t, failed)
	assert.False(t, wf.checkResult)

	eqPassed := controller.CheckEqual("eq_pass", 10, 10)
	assert.True(t, eqPassed)
	assert.True(t, wf.checkResult)

	eqFailed := controller.CheckEqual("eq_fail", 10, 20)
	assert.False(t, eqFailed)
	assert.False(t, wf.checkResult)

	truePassed := controller.CheckTrue("true_pass", true)
	assert.True(t, truePassed)
	assert.True(t, wf.checkResult)

	trueFailed := controller.CheckTrue("true_fail", false)
	assert.False(t, trueFailed)
	assert.False(t, wf.checkResult)

	noErrPassed := controller.CheckNoError("noerr_pass", nil)
	assert.True(t, noErrPassed)
	assert.True(t, wf.checkResult)

	noErrFailed := controller.CheckNoError("noerr_fail", assert.AnError)
	assert.False(t, noErrFailed)
	assert.False(t, wf.checkResult)
}
