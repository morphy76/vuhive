package config_test

import (
	"testing"
	"time"

	"github.com/morphy76/vuhive/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_RampingVUsScenarios(t *testing.T) {
	tests := []struct {
		name          string
		cfg           *config.Config
		expectedError string
		field         string
	}{
		{
			name: "valid ramping_vus configuration",
			cfg: &config.Config{
				Version: "1.0",
				Scenarios: map[string]config.ScenarioConfig{
					"spike_test": {
						Type:      config.ScenarioTypeRampingVUs,
						VUTimeout: 2 * time.Second,
						Stages: []config.StageConfig{
							{Target: 10, Duration: 30 * time.Second},
							{Target: 10, Duration: 1 * time.Minute},
							{Target: 50, Duration: 10 * time.Second},
							{Target: 50, Duration: 2 * time.Minute},
							{Target: 0, Duration: 30 * time.Second},
						},
					},
				},
			},
		},
		{
			name: "missing stages in ramping_vus",
			cfg: &config.Config{
				Version: "1.0",
				Scenarios: map[string]config.ScenarioConfig{
					"spike_test": {
						Type:      config.ScenarioTypeRampingVUs,
						VUTimeout: 2 * time.Second,
						Stages:    []config.StageConfig{},
					},
				},
			},
			expectedError: "at least one stage must be defined for ramping_vus scenario type",
			field:         "scenarios.spike_test.stages",
		},
		{
			name: "stage with negative target",
			cfg: &config.Config{
				Version: "1.0",
				Scenarios: map[string]config.ScenarioConfig{
					"spike_test": {
						Type:      config.ScenarioTypeRampingVUs,
						VUTimeout: 2 * time.Second,
						Stages: []config.StageConfig{
							{Target: -5, Duration: 10 * time.Second},
						},
					},
				},
			},
			expectedError: "must be >= 0",
			field:         "scenarios.spike_test.stages[0].target",
		},
		{
			name: "stage with zero or negative duration",
			cfg: &config.Config{
				Version: "1.0",
				Scenarios: map[string]config.ScenarioConfig{
					"spike_test": {
						Type:      config.ScenarioTypeRampingVUs,
						VUTimeout: 2 * time.Second,
						Stages: []config.StageConfig{
							{Target: 10, Duration: 0},
						},
					},
				},
			},
			expectedError: "must be > 0",
			field:         "scenarios.spike_test.stages[0].duration",
		},
		{
			name: "negative vu_timeout in ramping_vus",
			cfg: &config.Config{
				Version: "1.0",
				Scenarios: map[string]config.ScenarioConfig{
					"spike_test": {
						Type: config.ScenarioTypeRampingVUs,
						Stages: []config.StageConfig{
							{Target: 10, Duration: 10 * time.Second},
						},
						VUTimeout: -1 * time.Second,
					},
				},
			},
			expectedError: "must be >= 0",
			field:         "scenarios.spike_test.vu_timeout",
		},
		{
			name: "omitted vu_timeout in ramping_vus defaults to guard deadline",
			cfg: &config.Config{
				Version: "1.0",
				Scenarios: map[string]config.ScenarioConfig{
					"spike_test": {
						Type: config.ScenarioTypeRampingVUs,
						Stages: []config.StageConfig{
							{Target: 10, Duration: 10 * time.Second},
						},
					},
				},
			},
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.Validate(tt.cfg)
			if tt.expectedError != "" {
				require.Error(t, err)
				var valErr *config.ValidationError
				require.ErrorAs(t, err, &valErr)
				assert.Contains(t, valErr.Message, tt.expectedError)
				assert.Equal(t, tt.field, valErr.Field)
			} else {
				require.NoError(t, err)
				if tt.name == "omitted vu_timeout in ramping_vus defaults to guard deadline" {
					assert.Equal(t, config.DefaultGuardDeadline, tt.cfg.Scenarios["spike_test"].VUTimeout)
				}
			}
		})
	}
}
