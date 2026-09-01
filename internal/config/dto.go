package config

import (
	"fmt"
	"reflect"
	"time"

	"github.com/go-viper/mapstructure/v2"
)

// configDTO is the top-level configuration DTO loaded from YAML via mapstructure.
type configDTO struct {
	Version         string                       `mapstructure:"version"`
	DefaultScenario string                       `mapstructure:"default_scenario"`
	Scenarios       map[string]scenarioConfigDTO `mapstructure:"scenarios"`
}

// scenarioConfigDTO is the DTO for a single scenario configuration.
type scenarioConfigDTO struct {
	Type             ScenarioType         `mapstructure:"type"`
	VUs              int                  `mapstructure:"vus"`
	TargetTPS        int                  `mapstructure:"target_tps"`
	MaxVUs           int                  `mapstructure:"max_vus"`
	BurstBuffer      int                  `mapstructure:"burst_buffer"`
	Stages           []stageConfigDTO     `mapstructure:"stages"`
	RampUp           time.Duration        `mapstructure:"ramp_up"`
	RunPeriod        time.Duration        `mapstructure:"run_period"`
	RampDown         time.Duration        `mapstructure:"ramp_down"`
	Drain            time.Duration        `mapstructure:"drain"`
	DrainPeriod      time.Duration        `mapstructure:"drain_period"`
	VUTimeout        time.Duration        `mapstructure:"vu_timeout"`
	Params           map[string]string    `mapstructure:"params"`
	InteractionDelay *thinkTimeConfigDTO  `mapstructure:"interaction_delay"`
	ThinkTime        *thinkTimeConfigDTO  `mapstructure:"think_time"`
	Thresholds       []thresholdConfigDTO `mapstructure:"thresholds"`
	HTTP             *httpConfigDTO                  `mapstructure:"http"`
	HTTPClients      map[string]*httpConfigDTO       `mapstructure:"http_clients"`
}

// httpConfigDTO is the DTO for HTTP client configuration.
type httpConfigDTO struct {
	BaseURL        string            `mapstructure:"base_url"`
	Timeout        time.Duration     `mapstructure:"timeout"`
	Headers        map[string]string `mapstructure:"headers"`
	TLS            tlsConfigDTO      `mapstructure:"tls"`
	Pool           httpPoolConfigDTO `mapstructure:"pool"`
	DetailedTiming bool              `mapstructure:"detailed_timing"`
	MetricPrefix   string            `mapstructure:"metric_prefix"`
}

// tlsConfigDTO is the DTO for TLS configuration.
type tlsConfigDTO struct {
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify"`
}

// httpPoolConfigDTO is the DTO for HTTP connection pool configuration.
type httpPoolConfigDTO struct {
	MaxIdleConns        int           `mapstructure:"max_idle_conns"`
	MaxIdleConnsPerHost int           `mapstructure:"max_idle_conns_per_host"`
	IdleConnTimeout     time.Duration `mapstructure:"idle_conn_timeout"`
}

// stageConfigDTO is the DTO for a single stage in ramping_vus scenarios.
type stageConfigDTO struct {
	Target   int           `mapstructure:"target"`
	Duration time.Duration `mapstructure:"duration"`
}

// thinkTimeConfigDTO is the DTO for think time and interaction delay configurations.
type thinkTimeConfigDTO struct {
	Type     string        `mapstructure:"type"`
	Duration time.Duration `mapstructure:"duration"`
	Min      time.Duration `mapstructure:"min"`
	Max      time.Duration `mapstructure:"max"`
	Mean     time.Duration `mapstructure:"mean"`
	StdDev   time.Duration `mapstructure:"std_dev"`
}

// thresholdConfigDTO is the DTO for SLA threshold assertions.
type thresholdConfigDTO struct {
	Metric         string        `mapstructure:"metric"`
	Stat           string        `mapstructure:"stat"`
	Operator       string        `mapstructure:"operator"`
	Target         string        `mapstructure:"target"`
	OnNoData       string        `mapstructure:"on_no_data"`
	AbortOnFail    bool          `mapstructure:"abort_on_fail"`
	DelayAbortEval time.Duration `mapstructure:"delay_abort_eval"`
}

// toModel converts a configDTO to a pure domain Config model.
func (d *configDTO) toModel() *Config {
	if d == nil {
		return nil
	}
	cfg := &Config{
		Version:         d.Version,
		DefaultScenario: d.DefaultScenario,
	}
	if d.Scenarios != nil {
		cfg.Scenarios = make(map[string]ScenarioConfig, len(d.Scenarios))
		for k, sc := range d.Scenarios {
			cfg.Scenarios[k] = sc.toModel()
		}
	}
	return cfg
}

// toModel converts a stageConfigDTO to a pure domain StageConfig model.
func (d *stageConfigDTO) toModel() StageConfig {
	return StageConfig{
		Target:   d.Target,
		Duration: d.Duration,
	}
}

// toModel converts a scenarioConfigDTO to a pure domain ScenarioConfig model.
func (d *scenarioConfigDTO) toModel() ScenarioConfig {
	drain := d.Drain
	if drain == 0 && d.DrainPeriod != 0 {
		drain = d.DrainPeriod
	}
	sc := ScenarioConfig{
		Type:        d.Type,
		VUs:         d.VUs,
		TargetTPS:   d.TargetTPS,
		MaxVUs:      d.MaxVUs,
		BurstBuffer: d.BurstBuffer,
		RampUp:      d.RampUp,
		RunPeriod:   d.RunPeriod,
		RampDown:    d.RampDown,
		Drain:       drain,
		VUTimeout:   d.VUTimeout,
	}

	if d.Stages != nil {
		sc.Stages = make([]StageConfig, len(d.Stages))
		for i, st := range d.Stages {
			sc.Stages[i] = st.toModel()
		}
	}
	if d.Params != nil {
		sc.Params = make(map[string]string, len(d.Params))
		for k, v := range d.Params {
			sc.Params[k] = v
		}
	}
	if d.InteractionDelay != nil {
		sc.InteractionDelay = d.InteractionDelay.toModel()
	}
	if d.ThinkTime != nil {
		sc.ThinkTime = d.ThinkTime.toModel()
	}
	if d.Thresholds != nil {
		sc.Thresholds = make([]ThresholdConfig, len(d.Thresholds))
		for i, th := range d.Thresholds {
			sc.Thresholds[i] = th.toModel()
		}
	}
	if d.HTTP != nil {
		sc.HTTP = d.HTTP.toModel()
	}

	if d.HTTPClients != nil {
		sc.HTTPClients = make(map[string]*HTTPConfig, len(d.HTTPClients))
		for name, clientDTO := range d.HTTPClients {
			sc.HTTPClients[name] = clientDTO.toModel()
		}
	}
	return sc
}

// toModel converts a httpConfigDTO to a pure domain HTTPConfig model.
func (d *httpConfigDTO) toModel() *HTTPConfig {
	if d == nil {
		return nil
	}
	var headers map[string]string
	if d.Headers != nil {
		headers = make(map[string]string, len(d.Headers))
		for k, v := range d.Headers {
			headers[k] = v
		}
	}
	return &HTTPConfig{
		BaseURL:        d.BaseURL,
		Timeout:        d.Timeout,
		Headers:        headers,
		TLS:            TLSConfig{InsecureSkipVerify: d.TLS.InsecureSkipVerify},
		Pool:           HTTPPoolConfig{MaxIdleConns: d.Pool.MaxIdleConns, MaxIdleConnsPerHost: d.Pool.MaxIdleConnsPerHost, IdleConnTimeout: d.Pool.IdleConnTimeout},
		DetailedTiming: d.DetailedTiming,
		MetricPrefix:   d.MetricPrefix,
	}
}

// toModel converts a thinkTimeConfigDTO to a pure domain ThinkTimeConfig model.
func (d *thinkTimeConfigDTO) toModel() *ThinkTimeConfig {
	if d == nil {
		return nil
	}
	return &ThinkTimeConfig{
		Type:     d.Type,
		Duration: d.Duration,
		Min:      d.Min,
		Max:      d.Max,
		Mean:     d.Mean,
		StdDev:   d.StdDev,
	}
}

// toModel converts a thresholdConfigDTO to a pure domain ThresholdConfig model.
func (d *thresholdConfigDTO) toModel() ThresholdConfig {
	return ThresholdConfig{
		Metric:         d.Metric,
		Stat:           d.Stat,
		Operator:       d.Operator,
		Target:         d.Target,
		OnNoData:       d.OnNoData,
		AbortOnFail:    d.AbortOnFail,
		DelayAbortEval: d.DelayAbortEval,
	}
}

// durationDecodeHook returns a mapstructure decode hook that converts string values
// to time.Duration for fields typed as time.Duration.
func durationDecodeHook() mapstructure.DecodeHookFuncType {
	return func(from reflect.Type, to reflect.Type, data any) (any, error) {
		if from.Kind() != reflect.String {
			return data, nil
		}
		if to != reflect.TypeOf(time.Duration(0)) {
			return data, nil
		}

		s, ok := data.(string)
		if !ok {
			return data, nil
		}

		// Empty string means zero duration (for optional fields with default "0s").
		if s == "" {
			return time.Duration(0), nil
		}

		d, err := time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("cannot parse duration %q: %w", s, err)
		}
		return d, nil
	}
}
