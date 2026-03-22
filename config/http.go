package config

import (
	"crypto/tls"
	"time"
)

// BaseHTTP contains shared HTTP server settings used by transports in this repository.
// nolint:lll
type BaseHTTP struct {
	appSettings

	TLSConfig         *TLS          `toml:"tls"                 yaml:"tls"                 json:"tls"               env:"TLS"`
	ReadTimeout       time.Duration `toml:"read_timeout"        yaml:"read_timeout"        json:"readTimeout"       env:"READ_TIMEOUT"`
	WriteTimeout      time.Duration `toml:"write_timeout"       yaml:"write_timeout"       json:"writeTimeout"      env:"WRITE_TIMEOUT"`
	ReadHeaderTimeout time.Duration `toml:"read_header_timeout" yaml:"read_header_timeout" json:"readHeaderTimeout" env:"READ_HEADER_TIMEOUT"`
	IdleTimeout       time.Duration `toml:"idle_timeout"        yaml:"idle_timeout"        json:"idleTimeout"       env:"IDLE_TIMEOUT"`
	ShutdownTimeout   time.Duration `toml:"shutdown_timeout"    yaml:"shutdown_timeout"    json:"shutdownTimeout"   env:"SHUTDOWN_TIMEOUT"    default:"30s"`
	MaxHeaderBytes    int           `toml:"max_header_bytes"    yaml:"max_header_bytes"    json:"maxHeaderBytes"    env:"MAX_HEADER_BYTES"`
}

// Ops contains settings for OPS server.
// nolint:lll
type Ops struct {
	BaseHTTP `yaml:",inline" env:",squash"`

	Address        string `yaml:"address"         env:"ADDRESS"         toml:"address"         json:"address"         default:":8090"`
	MetricsPath    string `yaml:"metrics_path"    env:"METRICS_PATH"    toml:"metrics_path"    json:"metrics_path"    default:"/metrics"`
	ProfilePath    string `yaml:"profile_path"    env:"PROFILE_PATH"    toml:"profile_path"    json:"profile_path"    default:"/debug/pprof"`
	ExpVarsPath    string `yaml:"exp_vars_path"   env:"EXP_VARS_PATH"   toml:"exp_vars_path"   json:"exp_vars_path"   default:"/debug/vars"`
	VersionPath    string `yaml:"version_path"    env:"VERSION_PATH"    toml:"version_path"    json:"version_path"    default:"/version"`
	VersionEnabled bool   `yaml:"version_enabled" env:"VERSION_ENABLED" toml:"version_enabled" json:"version_enabled" default:"false"`
}

// HTTPConfig provides the address and base server settings required by network/http.
type HTTPConfig interface {
	Addr() string
	Base() BaseHTTP
}

// Base returns the underlying BaseHTTP value.
func (c BaseHTTP) Base() BaseHTTP { return c }

// Addr returns the configured listen address.
func (c Ops) Addr() string { return c.Address }

// PrepareTLSConfig builds a tls.Config from BaseHTTP.TLSConfig.
func (c BaseHTTP) PrepareTLSConfig() (*tls.Config, error) {
	if c.TLSConfig == nil {
		return nil, ErrTLSDisabled
	}

	return c.TLSConfig.Prepare()
}
