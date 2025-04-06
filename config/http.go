package config

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/im-kulikov/go-bones"
)

type TLS struct {
	Enabled      bool     `toml:"enabled" yaml:"enabled" json:"enabled" env:"ENABLED" default:"false"`
	CertFile     string   `toml:"cert_file" yaml:"cert_file" json:"certFile" env:"CERT_FILE"`
	KeyFile      string   `toml:"key_file" yaml:"key_file" json:"keyFile" env:"KEY_FILE"`
	ClientAuth   string   `toml:"client_auth" yaml:"client_auth" json:"clientAuth" env:"CLIENT_AUTH" default:"no-client-cert"`
	CACertFile   string   `toml:"ca_cert_file" yaml:"ca_cert_file" json:"CACertFile" env:"CA_CERT_FILE"`
	MinVersion   string   `toml:"min_version" yaml:"min_version" json:"minVersion" env:"MIN_VERSION" default:"TLS13"`
	CipherSuites []string `toml:"cipher_suites" yaml:"cipher_suites" json:"cipherSuites" env:"CIPHER_SUITES"`
}

type BaseHTTP struct {
	appSettings

	TLSConfig         *TLS          `toml:"tls" yaml:"tls" json:"tls" env:"TLS"`
	ReadTimeout       time.Duration `toml:"read_timeout" yaml:"read_timeout" json:"readTimeout" env:"READ_TIMEOUT"`
	WriteTimeout      time.Duration `toml:"write_timeout" yaml:"write_timeout" json:"writeTimeout" env:"WRITE_TIMEOUT"`
	ReadHeaderTimeout time.Duration `toml:"read_header_timeout" yaml:"read_header_timeout" json:"readHeaderTimeout" env:"READ_HEADER_TIMEOUT"`
	IdleTimeout       time.Duration `toml:"idle_timeout" yaml:"idle_timeout" json:"idleTimeout" env:"IDLE_TIMEOUT"`
	MaxHeaderBytes    int           `toml:"max_header_bytes" yaml:"max_header_bytes" json:"maxHeaderBytes" env:"MAX_HEADER_BYTES"`
}

type HTTP struct {
	BaseHTTP `yaml:",inline" env:",squash"`

	Address string `json:"address" yaml:"address" env:"ADDRESS"`
}

type Ops struct {
	BaseHTTP `yaml:",inline" env:",squash"`

	Address     string `yaml:"address" json:"address" env:"ADDRESS" default:":8090"`
	MetricsPath string `json:"metrics_path" yaml:"metrics_path" env:"METRICS_PATH" default:"/metrics"`
	ProfilePath string `json:"profile_path" yaml:"profile_path" env:"PROFILE_PATH" default:"/debug/pprof"`
}

type HTTPConfig interface {
	Addr() string
	Base() BaseHTTP
	PrepareHTTPServer() (*http.Server, error)
}

const (
	ErrTLSDisabled          bones.Error = "TLS disabled"
	ErrTLSEmptyKeyPair      bones.Error = "TLS empty keypair"
	ErrUnknownTLSVersion    bones.Error = "unknown TLS version"
	ErrUnknownTLSClientAuth bones.Error = "unknown TLS client auth type"
	ErrTLSLoadX509KeyPair   bones.Error = "could not load X509 key pair"
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

func (c BaseHTTP) Base() BaseHTTP { return c }

func (c Ops) Addr() string { return c.Address }

func (c HTTP) Addr() string { return c.Address }

func (c Ops) PrepareHTTPServer() (*http.Server, error) { return httpServerSettings(c) }

func (c HTTP) PrepareHTTPServer() (*http.Server, error) { return httpServerSettings(c) }

func (c BaseHTTP) PrepareTLSConfig() (*tls.Config, error) {
	if c.TLSConfig == nil || !c.TLSConfig.Enabled {
		return nil, ErrTLSDisabled
	}

	if c.TLSConfig.CertFile == "" || c.TLSConfig.KeyFile == "" {
		return nil, fmt.Errorf("%w: cert=%q, key=%q", ErrTLSEmptyKeyPair, c.TLSConfig.CertFile, c.TLSConfig.KeyFile)
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

func httpServerSettings(c HTTPConfig) (*http.Server, error) {
	var err error
	base := c.Base()

	var cfg *tls.Config
	if cfg, err = base.PrepareTLSConfig(); err != nil && !errors.Is(err, ErrTLSDisabled) {
		return nil, err
	}

	return &http.Server{
		Addr:              c.Addr(),
		TLSConfig:         cfg,
		ReadTimeout:       base.ReadTimeout,
		ReadHeaderTimeout: base.ReadHeaderTimeout,
		WriteTimeout:      base.WriteTimeout,
		IdleTimeout:       base.IdleTimeout,
		MaxHeaderBytes:    base.MaxHeaderBytes,
	}, nil
}
