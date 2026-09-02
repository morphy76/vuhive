package cli_test

import (
	"bytes"
	"testing"

	"github.com/morphy76/vuhive/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlagsDefaults(t *testing.T) {
	var errBuf bytes.Buffer
	flags, err := cli.ParseFlags([]string{}, &errBuf)
	require.NoError(t, err)

	assert.Equal(t, "vuhive.yaml", flags.ConfigPath)
	assert.Equal(t, "", flags.ScenarioName)
	assert.Equal(t, "info", flags.LogLevel)
	assert.Equal(t, "pretty", flags.LogFormat)
	assert.Equal(t, "console", flags.ReportFormat)
	assert.Equal(t, "", flags.ReportOut)
	assert.False(t, flags.ShowVersion)
	assert.False(t, flags.Strict)
	assert.False(t, flags.StrictFatal)
}

func TestParseFlagsCustomValues(t *testing.T) {
	var errBuf bytes.Buffer
	args := []string{
		"--config", "custom.yaml",
		"--scenario", "checkout",
		"--log-level", "debug",
		"--log-format", "json",
		"--report-format", "json",
		"--report-out", "report.json",
		"--version",
		"--strict",
		"--strict-fatal",
	}

	flags, err := cli.ParseFlags(args, &errBuf)
	require.NoError(t, err)

	assert.Equal(t, "custom.yaml", flags.ConfigPath)
	assert.Equal(t, "checkout", flags.ScenarioName)
	assert.Equal(t, "debug", flags.LogLevel)
	assert.Equal(t, "json", flags.LogFormat)
	assert.Equal(t, "json", flags.ReportFormat)
	assert.Equal(t, "report.json", flags.ReportOut)
	assert.True(t, flags.ShowVersion)
	assert.True(t, flags.Strict)
	assert.True(t, flags.StrictFatal)
}

func TestParseFlagsInvalidFlagReturnsError(t *testing.T) {
	var errBuf bytes.Buffer
	_, err := cli.ParseFlags([]string{"--unknown-flag"}, &errBuf)
	require.Error(t, err)
}
