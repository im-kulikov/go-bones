package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"testing"
	"time"

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

func Test_TLSErrors(t *testing.T) {
	cases := []struct {
		name   string
		conf   TLS
		expect error
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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.conf.Prepare()
			require.ErrorIs(t, err, tc.expect)
		})
	}
}
