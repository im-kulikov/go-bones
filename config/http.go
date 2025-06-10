package config

import (
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/im-kulikov/go-bones"
)

// nolint:lll
type TLS struct {
	Enabled      bool     `toml:"enabled"       yaml:"enabled"       json:"enabled"      env:"ENABLED"       default:"false"`
	CertFile     string   `toml:"cert_file"     yaml:"cert_file"     json:"certFile"     env:"CERT_FILE"`
	KeyFile      string   `toml:"key_file"      yaml:"key_file"      json:"keyFile"      env:"KEY_FILE"`
	ClientAuth   string   `toml:"client_auth"   yaml:"client_auth"   json:"clientAuth"   env:"CLIENT_AUTH"   default:"no-client-cert"`
	CACertFile   string   `toml:"ca_cert_file"  yaml:"ca_cert_file"  json:"CACertFile"   env:"CA_CERT_FILE"`
	MinVersion   string   `toml:"min_version"   yaml:"min_version"   json:"minVersion"   env:"MIN_VERSION"   default:"TLS13"`
	CipherSuites []string `toml:"cipher_suites" yaml:"cipher_suites" json:"cipherSuites" env:"CIPHER_SUITES"`
}

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
	BaseHTTP    `       yaml:",inline"       env:",squash"`
	Address     string `yaml:"address"       env:"ADDRESS"       toml:"address"       json:"address"       default:":8090"`
	MetricsPath string `yaml:"metrics_path"  env:"METRICS_PATH"  toml:"metrics_path"  json:"metrics_path"  default:"/metrics"`
	ProfilePath string `yaml:"profile_path"  env:"PROFILE_PATH"  toml:"profile_path"  json:"profile_path"  default:"/debug/pprof"`
	ExpVarsPath string `yaml:"exp_vars_path" env:"EXP_VARS_PATH" toml:"exp_vars_path" json:"exp_vars_path" default:"/debug/vars"`
}

// HTTPConfig an interface for http settings.
type HTTPConfig interface {
	Addr() string
	Base() BaseHTTP
}

const (
	// ErrTLSDisabled fires when tls disabled.
	ErrTLSDisabled bones.Error = "TLS disabled"

	// ErrTLSEmptyKeyPair fires when TLS.CertFile or TLS.KeyFile is empty.
	ErrTLSEmptyKeyPair bones.Error = "TLS empty keypair"

	// ErrUnknownTLSVersion fires when TLS.MinVersion is unknown.
	ErrUnknownTLSVersion bones.Error = "unknown TLS version"

	// ErrUnknownTLSClientAuth fires when TLS.ClientAuth is unknown.
	ErrUnknownTLSClientAuth bones.Error = "unknown TLS client auth type"

	// ErrTLSLoadX509KeyPair fires when could not load tls.X509KeyPair.
	ErrTLSLoadX509KeyPair bones.Error = "could not load X509 key pair"
)

// nolint:gochecknoglobals
var clientAuthMap = map[string]tls.ClientAuthType{
	"no-client-cert":                 tls.NoClientCert,
	"request-client-cert":            tls.RequestClientCert,
	"require-any-client-cert":        tls.RequireAnyClientCert,
	"verify-client-cert-if-given":    tls.VerifyClientCertIfGiven,
	"require-and-verify-client-cert": tls.RequireAndVerifyClientCert,
}

// nolint:gochecknoglobals
var tlsVersions = map[string]uint16{
	"TLS13": tls.VersionTLS13,
	"TLS12": tls.VersionTLS12,
	"TLS11": tls.VersionTLS11,
	"TLS10": tls.VersionTLS10,
}

// Base settings for http.Server.
func (c BaseHTTP) Base() BaseHTTP { return c }

// Addr for http.Server.
func (c Ops) Addr() string { return c.Address }

// PrepareTLSConfig creates tls.Config from settings.
func (c BaseHTTP) PrepareTLSConfig() (*tls.Config, error) {
	if c.TLSConfig == nil || !c.TLSConfig.Enabled {
		return nil, ErrTLSDisabled
	}

	if c.TLSConfig.CertFile == "" || c.TLSConfig.KeyFile == "" {
		return nil, fmt.Errorf(
			"%w: cert=%q, key=%q",
			ErrTLSEmptyKeyPair,
			c.TLSConfig.CertFile,
			c.TLSConfig.KeyFile,
		)
	}

	var err error
	if _, ok := tlsVersions[c.TLSConfig.MinVersion]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownTLSVersion, c.TLSConfig.MinVersion)
	}

	if _, ok := clientAuthMap[c.TLSConfig.ClientAuth]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownTLSClientAuth, c.TLSConfig.ClientAuth)
	}

	var certificates [1]tls.Certificate
	if certificates[0], err = tls.LoadX509KeyPair(c.TLSConfig.CertFile, c.TLSConfig.KeyFile); err != nil {
		return nil, errors.Join(ErrTLSLoadX509KeyPair, err)
	}

	var minVersion uint16 = tls.VersionTLS13
	if tmp, ok := tlsVersions[c.TLSConfig.MinVersion]; ok {
		minVersion = tmp
	}

	return &tls.Config{
		Certificates: certificates[:],
		ClientAuth:   clientAuthMap[c.TLSConfig.ClientAuth],
		MinVersion:   minVersion,
	}, nil
}
