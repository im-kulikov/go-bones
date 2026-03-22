package config

import (
	"crypto/tls"
	"time"
)

// BaseGRPC contains shared gRPC server settings used by network/grpc.
// nolint:lll
type BaseGRPC struct {
	appSettings

	TLSConfig         *TLS          `toml:"tls"                 yaml:"tls"                 json:"tls"               env:"TLS"`
	ReadTimeout       time.Duration `toml:"read_timeout"        yaml:"read_timeout"        json:"readTimeout"       env:"READ_TIMEOUT"`
	WriteTimeout      time.Duration `toml:"write_timeout"       yaml:"write_timeout"       json:"writeTimeout"      env:"WRITE_TIMEOUT"`
	ReadHeaderTimeout time.Duration `toml:"read_header_timeout" yaml:"read_header_timeout" json:"readHeaderTimeout" env:"READ_HEADER_TIMEOUT"`
	IdleTimeout       time.Duration `toml:"idle_timeout"        yaml:"idle_timeout"        json:"idleTimeout"       env:"IDLE_TIMEOUT"`
	ShutdownTimeout   time.Duration `toml:"shutdown_timeout"    yaml:"shutdown_timeout"    json:"shutdownTimeout"   env:"SHUTDOWN_TIMEOUT"    default:"30s"`
	MaxHeaderBytes    int           `toml:"max_header_bytes"    yaml:"max_header_bytes"    json:"maxHeaderBytes"    env:"MAX_HEADER_BYTES"`
}

// GRPCConfig is an interface defining configuration options for a gRPC server.
// Addr returns the address the gRPC server will listen to.
// Base returns the BaseGRPC configuration struct for detailed settings.
type GRPCConfig interface {
	Addr() string
	Base() BaseGRPC
}

// Base returns the underlying BaseGRPC value.
func (c BaseGRPC) Base() BaseGRPC { return c }

// PrepareTLSConfig builds a tls.Config from BaseGRPC.TLSConfig.
func (c BaseGRPC) PrepareTLSConfig() (*tls.Config, error) {
	if c.TLSConfig == nil {
		return nil, ErrTLSDisabled
	}

	return c.TLSConfig.Prepare()
}
