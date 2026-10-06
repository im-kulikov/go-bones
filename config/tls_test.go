package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/im-kulikov/gonfig"
	"github.com/stretchr/testify/require"
)

func generateTLSKeyPair(t *testing.T) (string, string) {
	t.Helper()

	private, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	serialNumber, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	require.NoError(t, err)

	tpl := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(time.Hour * 24),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, &tpl, &tpl, &private.PublicKey, private)
	require.NoError(t, err)

	data := x509.MarshalPKCS1PrivateKey(private)
	keyFile, err := os.CreateTemp(t.TempDir(), "*.key")
	require.NoError(t, err)
	require.NoError(t, pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: data}))
	require.NoError(t, keyFile.Close())

	crtFile, err := os.CreateTemp(t.TempDir(), "*.crt")
	require.NoError(t, err)
	require.NoError(t, pem.Encode(crtFile, &pem.Block{Type: "CERTIFICATE", Bytes: der}))
	require.NoError(t, crtFile.Close())

	return keyFile.Name(), crtFile.Name()
}

// Test_TLSErrors covers every error path through TLS.Prepare with a single
// table. Cases that only need "it errored" use contains; cases that need to
// pin down a specific sentinel use expect (both may be set).
func Test_TLSErrors(t *testing.T) {
	keyFile, certFile := generateTLSKeyPair(t)

	invalidCAFile := t.TempDir() + "/invalid-ca.pem"
	require.NoError(t, os.WriteFile(invalidCAFile, []byte("not a PEM certificate"), 0o600))

	cases := []struct {
		name     string
		conf     TLS
		expect   error
		contains string
	}{
		{name: "disabled", conf: TLS{}, expect: ErrTLSDisabled},
		{
			name:   "empty keypair",
			conf:   TLS{Enabled: true},
			expect: ErrTLSEmptyKeyPair,
		},
		{
			name: "wrong min version",
			conf: TLS{
				Enabled:  true,
				CertFile: "file.crt",
				KeyFile:  "file.key",
			},
			expect: ErrUnknownTLSVersion,
		},
		{
			name: "wrong client auth",
			conf: TLS{
				Enabled:    true,
				CertFile:   "file.crt",
				KeyFile:    "file.key",
				MinVersion: "TLS13",
			},
			expect: ErrUnknownTLSClientAuth,
		},
		{
			name: "cipher_suites ineffective at TLS13",
			conf: TLS{
				Enabled:      true,
				CertFile:     "file.crt",
				KeyFile:      "file.key",
				MinVersion:   "TLS13",
				ClientAuth:   "no-client-cert",
				CipherSuites: []string{tls.CipherSuites()[0].Name},
			},
			expect: ErrCipherSuitesIneffectiveAtTLS13,
		},
		{
			name: "could not load key pair",
			conf: TLS{
				Enabled:    true,
				CertFile:   "file.crt",
				KeyFile:    "file.key",
				MinVersion: "TLS13",
				ClientAuth: "no-client-cert",
			},
			expect: ErrTLSLoadX509KeyPair,
		},
		{
			name: "unknown cipher suite",
			conf: TLS{
				Enabled:      true,
				CertFile:     certFile,
				KeyFile:      keyFile,
				ClientAuth:   "no-client-cert",
				MinVersion:   "TLS12",
				CipherSuites: []string{"TLS_NOT_A_REAL_CIPHER_SUITE"},
			},
			contains: "unknown or unsupported TLS cipher suite",
		},
		{
			name: "client cert verification requires CA (verify-if-given)",
			conf: TLS{
				Enabled:    true,
				CertFile:   certFile,
				KeyFile:    keyFile,
				ClientAuth: "verify-client-cert-if-given",
				MinVersion: "TLS13",
			},
			expect: ErrMTLSRequiresCACertFile,
		},
		{
			name: "client cert verification requires CA (require-and-verify)",
			conf: TLS{
				Enabled:    true,
				CertFile:   certFile,
				KeyFile:    keyFile,
				ClientAuth: "require-and-verify-client-cert",
				MinVersion: "TLS13",
			},
			expect: ErrMTLSRequiresCACertFile,
		},
		{
			name: "invalid client CA PEM",
			conf: TLS{
				Enabled:    true,
				CertFile:   certFile,
				KeyFile:    keyFile,
				CACertFile: invalidCAFile,
				ClientAuth: "require-and-verify-client-cert",
				MinVersion: "TLS13",
			},
			contains: "could not parse client ca",
		},
		{
			name: "client CA file read error",
			conf: TLS{
				Enabled:    true,
				CertFile:   certFile,
				KeyFile:    keyFile,
				CACertFile: t.TempDir() + "/missing-ca.pem",
				ClientAuth: "no-client-cert",
				MinVersion: "TLS13",
			},
			contains: "could not load client ca file",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.conf.Prepare()
			require.Error(t, err)

			if tc.expect != nil {
				require.ErrorIs(t, err, tc.expect)
			}

			if tc.contains != "" {
				require.ErrorContains(t, err, tc.contains)
			}
		})
	}
}

// TestTLSPrepare_ClientCAAndCipherSuites is the success-path counterpart to
// Test_TLSErrors: proves mTLS + cipher_suites configure cleanly below TLS 1.3.
func TestTLSPrepare_ClientCAAndCipherSuites(t *testing.T) {
	keyFile, certFile := generateTLSKeyPair(t)
	suite := tls.CipherSuites()[0]

	cfg, err := TLS{
		Enabled:      true,
		CertFile:     certFile,
		KeyFile:      keyFile,
		CACertFile:   certFile,
		ClientAuth:   "require-and-verify-client-cert",
		MinVersion:   "TLS12",
		CipherSuites: []string{suite.Name},
	}.Prepare()
	require.NoError(t, err)
	require.Equal(t, tls.RequireAndVerifyClientCert, cfg.ClientAuth)
	require.NotNil(t, cfg.ClientCAs)
	require.Equal(t, []uint16{suite.ID}, cfg.CipherSuites)
}

func TestCipherSuiteIDs(t *testing.T) {
	suite := tls.CipherSuites()[0]

	ids, err := cipherSuiteIDs([]string{suite.Name})
	require.NoError(t, err)
	require.Equal(t, []uint16{suite.ID}, ids)

	ids, err = cipherSuiteIDs(nil)
	require.NoError(t, err)
	require.Nil(t, ids)

	_, err = cipherSuiteIDs([]string{"TLS_NOT_A_REAL_CIPHER_SUITE"})
	require.Error(t, err)
}

// A pointer TLS section set only in a file starts from its default tags too
// (gonfig v0.7.0), so Prepare works without min_version and client_auth.
func TestNetwork_TLSFromFileUsesDefaults(t *testing.T) {
	keyFile, certFile := generateTLSKeyPair(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "api:\n  tls:\n    enabled: true\n    cert_file: " + certFile + "\n    key_file: " + keyFile + "\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	var cfg struct {
		DefaultConfigFlag

		API HTTP `yaml:"api"`
	}

	require.NoError(t, Load(&cfg, WithCustomizeLoaderConfig(func(c *gonfig.Config) {
		c.Args, c.Envs = []string{"--config", path}, []string{}
	})))

	out, err := cfg.API.PrepareTLSConfig()
	require.NoError(t, err)
	require.Equal(t, uint16(tls.VersionTLS13), out.MinVersion)
	require.Equal(t, tls.NoClientCert, out.ClientAuth)
}
