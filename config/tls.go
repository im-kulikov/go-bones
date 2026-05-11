package config

import (
	"crypto/tls"
	"errors"
	"fmt"

	"github.com/im-kulikov/go-bones"
)

// TLS represents the configuration settings for enabling and managing TLS encryption in the application.
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

// Prepare initializes and returns a tls.Config based on the TLS settings, or an error if the configuration is invalid.
func (c TLS) Prepare() (*tls.Config, error) {
	if !c.Enabled {
		return nil, ErrTLSDisabled
	}

	if c.CertFile == "" || c.KeyFile == "" {
		return nil, fmt.Errorf(
			"%w: cert=%q, key=%q",
			ErrTLSEmptyKeyPair,
			c.CertFile,
			c.KeyFile,
		)
	}

	var err error
	if _, ok := tlsVersions[c.MinVersion]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownTLSVersion, c.MinVersion)
	}

	if _, ok := clientAuthMap[c.ClientAuth]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownTLSClientAuth, c.ClientAuth)
	}

	var certificates [1]tls.Certificate
	if certificates[0], err = tls.LoadX509KeyPair(c.CertFile, c.KeyFile); err != nil {
		return nil, errors.Join(ErrTLSLoadX509KeyPair, err)
	}

	var minVersion uint16 = tls.VersionTLS13
	if tmp, ok := tlsVersions[c.MinVersion]; ok {
		minVersion = tmp
	}

	return &tls.Config{
		Certificates: certificates[:],
		ClientAuth:   clientAuthMap[c.ClientAuth],
		MinVersion:   minVersion,
	}, nil
}
