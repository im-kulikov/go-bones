package config

type Logger struct {
	appSettings

	// OpenTracingEnabled allows to enable opentracing for logger.
	// nolint:lll
	OpenTracingEnabled bool `env:"OPEN_TRACING_ENABLED" yaml:"open-tracing" json:"open-tracing" toml:"open-tracing" default:"false"`

	// Secrets allows to set slice of log fields that should be marked as REDACTED.
	Secrets []string `env:"SECRETS" yaml:"secrets" json:"secrets" toml:"secrets"`
}
