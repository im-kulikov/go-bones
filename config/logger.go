package config

// Logger defines configuration parameters for the application logger.
// It allows configuring log level, output format, secret fields masking,
// and enabling OpenTracing integration.
//
// - OpenTracingEnabled allows enabling opentracing for logger.
//
// - Secrets allow setting slice of log fields that should be marked as REDACTED.
//
// - Level allows setting the default logger level.
//
// - Format allows settings the default logger format.
//
// nolint:lll
type Logger struct {
	appSettings

	OpenTracingEnabled bool     `env:"OPEN_TRACING_ENABLED" yaml:"open_tracing" json:"open_tracing" toml:"open_tracing" default:"false"`
	Secrets            []string `env:"SECRETS"              yaml:"secrets"      json:"secrets"      toml:"secrets"`
	Level              string   `env:"LEVEL"                yaml:"level"        json:"level"        toml:"level"        default:"info"  usage:"Allows to set level for default logger"`
	Format             string   `env:"FORMAT"               yaml:"format"       json:"format"       toml:"format"       default:"text"  usage:"Allows to set format for default logger"`
	AddSource          bool     `env:"ADD_SOURCE"           yaml:"add_source"   json:"add_source"   toml:"add_source"`
	AddAppInfo         bool     `env:"ADD_APP_INFO"         yaml:"add_app_info" json:"add_app_info" toml:"add_app_info"`
}
