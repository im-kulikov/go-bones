package config

import "time"

// Health contains settings for the health monitor (package health).
//
// The values are starting points, not an SLA: every dependency can override
// interval, timeout and thresholds with health options at registration time.
//
//nolint:lll,golines // Struct tags keep all supported config backends visible per field.
type Health struct {
	// Interval is the period between polled checks once a check has passed at least once.
	Interval time.Duration `yaml:"interval" env:"INTERVAL" toml:"interval" json:"interval" default:"10s"`
	// InitialInterval is the period used until a check reports its first success.
	InitialInterval time.Duration `yaml:"initial_interval" env:"INITIAL_INTERVAL" toml:"initial_interval" json:"initial_interval" default:"1s"`
	// Timeout is the deadline of a single Check call.
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT" toml:"timeout" json:"timeout" default:"2s"`
	// MinInterval is the minimal distance between two runs requested by Trigger.
	MinInterval time.Duration `yaml:"min_interval" env:"MIN_INTERVAL" toml:"min_interval" json:"min_interval" default:"1s"`
	// StaleAfter marks a polled result as stale; 0 means 2*Interval + Timeout.
	StaleAfter time.Duration `yaml:"stale_after" env:"STALE_AFTER" toml:"stale_after" json:"stale_after"`
	// FailureThreshold is the number of consecutive failures that turns a passing check into failing.
	FailureThreshold int `yaml:"failure_threshold" env:"FAILURE_THRESHOLD" toml:"failure_threshold" json:"failure_threshold" default:"1"`
	// SuccessThreshold is the number of consecutive successes that turns a failing check into passing.
	SuccessThreshold int `yaml:"success_threshold" env:"SUCCESS_THRESHOLD" toml:"success_threshold" json:"success_threshold" default:"1"`
	// DrainDelay is the pause between marking the service not ready and stopping services.
	// Keep it 0 when Kubernetes preStop.sleep is used, otherwise both delays add up.
	DrainDelay time.Duration `yaml:"drain_delay" env:"DRAIN_DELAY" toml:"drain_delay" json:"drain_delay"`
	// LogRepeatInterval limits how often a still failing check is reminded in logs.
	LogRepeatInterval time.Duration `yaml:"log_repeat_interval" env:"LOG_REPEAT_INTERVAL" toml:"log_repeat_interval" json:"log_repeat_interval" default:"5m"`
}

// Default health values, used by health.New for zero fields of Health.
const (
	DefaultHealthInterval          = 10 * time.Second
	DefaultHealthInitialInterval   = time.Second
	DefaultHealthTimeout           = 2 * time.Second
	DefaultHealthMinInterval       = time.Second
	DefaultHealthLogRepeatInterval = 5 * time.Minute
)

// WithDefaults returns a copy of Health where zero (or negative) values are
// replaced by defaults. It lets callers use Health{} without loading config.
func (h Health) WithDefaults() Health {
	if h.Interval <= 0 {
		h.Interval = DefaultHealthInterval
	}

	if h.InitialInterval <= 0 {
		h.InitialInterval = DefaultHealthInitialInterval
	}

	if h.Timeout <= 0 {
		h.Timeout = DefaultHealthTimeout
	}

	if h.MinInterval <= 0 {
		h.MinInterval = DefaultHealthMinInterval
	}

	if h.FailureThreshold < 1 {
		h.FailureThreshold = 1
	}

	if h.SuccessThreshold < 1 {
		h.SuccessThreshold = 1
	}

	if h.LogRepeatInterval <= 0 {
		h.LogRepeatInterval = DefaultHealthLogRepeatInterval
	}

	if h.DrainDelay < 0 {
		h.DrainDelay = 0
	}

	if h.StaleAfter < 0 {
		h.StaleAfter = 0
	}

	return h
}
