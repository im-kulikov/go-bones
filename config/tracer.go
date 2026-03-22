package config

// TracerConfig provides repository-local fallback settings for OpenTelemetry bootstrap.
//
// Standard OTEL_* environment variables remain the primary configuration contract.
// These fields exist as a convenience layer for local defaults and compatibility
// with repository-level configuration.
// nolint:lll
type TracerConfig struct {
	appSettings

	Enabled     bool   `yaml:"enabled"      json:"enabled"      toml:"enabled"      env:"ENABLED"      default:"false"          usage:"allows to enable OpenTelemetry bootstrap"`
	SendLogs    bool   `yaml:"send_logs"    json:"send_logs"    toml:"send_logs"    env:"SEND_LOGS"    default:"false"          usage:"allows to send OpenTelemetry logs"`
	SendMetrics bool   `yaml:"send_metrics" json:"send_metrics" toml:"send_metrics" env:"SEND_METRICS" default:"false"          usage:"allows to send OpenTelemetry metrics"`
	Endpoint    string `yaml:"endpoint"     json:"endpoint"     toml:"endpoint"     env:"ENDPOINT"     default:"localhost:4317" usage:"allows to set OTLP endpoint as a fallback when OTEL exporter env is absent"`
	Insecure    bool   `yaml:"insecure"     json:"insecure"     toml:"insecure"     env:"INSECURE"     default:"true"           usage:"allows to disable OTLP TLS as a fallback when OTEL exporter env is absent"`
	UseHTTP     bool   `yaml:"use_http"     json:"use_http"     toml:"use_http"     env:"USE_HTTP"     default:"false"          usage:"allows to use OTLP HTTP as a fallback when OTEL exporter protocol env is absent"`
}
