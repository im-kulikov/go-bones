package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

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

	// ErrMTLSRequiresCACertFile fires when ca_cert_file is empty but client auth requires it.
	ErrMTLSRequiresCACertFile bones.Error = "mTLS requires ca_cert_file"

	// ErrCipherSuitesIneffectiveAtTLS13 fires when cipher_suites is set together with
	// min_version=TLS13: Go's TLS 1.3 stack uses its own fixed, non-configurable
	// cipher suite set and silently ignores tls.Config.CipherSuites, so the setting
	// would give a false sense of control over the negotiated cipher.
	ErrCipherSuitesIneffectiveAtTLS13 bones.Error = "cipher_suites has no effect at TLS 1.3"
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

func cipherSuiteIDs(names []string) ([]uint16, error) {
	if len(names) == 0 {
		return nil, nil // use Go's secure default cipher suite set
	}

	known := make(map[string]uint16, len(tls.CipherSuites()))
	for _, suite := range tls.CipherSuites() {
		known[suite.Name] = suite.ID
	}

	ids := make([]uint16, 0, len(names))
	for _, name := range names {
		id, ok := known[name]
		if !ok {
			return nil, fmt.Errorf("unknown or unsupported TLS cipher suite %q", name)
		}

		ids = append(ids, id)
	}

	return ids, nil
}

func validateClientCA(authType tls.ClientAuthType, caFile string) error {
	requireCA := authType == tls.VerifyClientCertIfGiven ||
		authType == tls.RequireAndVerifyClientCert
	if requireCA && caFile == "" {
		return fmt.Errorf("%w: auth type = %q", ErrMTLSRequiresCACertFile, authType)
	}

	return nil
}

// Prepare initializes and returns a tls.Config based on the TLS settings, or an error if the configuration is invalid.
func (c TLS) Prepare() (*tls.Config, error) {
	minVersion, err := c.validate()
	if err != nil {
		return nil, err
	}

	var certificates [1]tls.Certificate
	if certificates[0], err = tls.LoadX509KeyPair(c.CertFile, c.KeyFile); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTLSLoadX509KeyPair, err)
	}

	cfg := &tls.Config{
		Certificates: certificates[:],
		ClientAuth:   clientAuthMap[c.ClientAuth],
		MinVersion:   minVersion,
	}

	if cfg.CipherSuites, err = cipherSuiteIDs(c.CipherSuites); err != nil {
		return nil, err
	}

	if err = validateClientCA(cfg.ClientAuth, c.CACertFile); err != nil {
		return nil, err
	}

	if cfg.ClientCAs, err = loadClientCAs(c.CACertFile); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate checks the settings that do not require reading files and returns
// the resolved minimum TLS version.
func (c TLS) validate() (uint16, error) {
	if !c.Enabled {
		return 0, ErrTLSDisabled
	}

	if c.CertFile == "" || c.KeyFile == "" {
		return 0, fmt.Errorf(
			"%w: cert=%q, key=%q",
			ErrTLSEmptyKeyPair,
			c.CertFile,
			c.KeyFile,
		)
	}

	minVersion, ok := tlsVersions[c.MinVersion]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrUnknownTLSVersion, c.MinVersion)
	}

	if _, ok = clientAuthMap[c.ClientAuth]; !ok {
		return 0, fmt.Errorf("%w: %s", ErrUnknownTLSClientAuth, c.ClientAuth)
	}

	if minVersion == tls.VersionTLS13 && len(c.CipherSuites) > 0 {
		return 0, fmt.Errorf("%w: min_version=%s, cipher_suites=%v",
			ErrCipherSuitesIneffectiveAtTLS13, c.MinVersion, c.CipherSuites)
	}

	return minVersion, nil
}

// loadClientCAs reads the PEM bundle used to verify client certificates.
// An empty path means no client CA pool.
func loadClientCAs(caFile string) (*x509.CertPool, error) {
	if caFile == "" {
		return nil, nil
	}

	pem, err := os.ReadFile(caFile) // #nosec G304 -- trusted config path
	if err != nil {
		return nil, fmt.Errorf("could not load client ca file (%q): %w", caFile, err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("could not parse client ca: invalid PEM")
	}

	return pool, nil
}
