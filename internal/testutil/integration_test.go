package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type attrRecorderTB struct {
	testing.TB
	attrs map[string]string
}

func (t *attrRecorderTB) Attr(key, value string) {
	if t.attrs == nil {
		t.attrs = make(map[string]string)
	}

	t.attrs[key] = value
}

type artifactTB struct {
	testing.TB
	dir string
}

func (t artifactTB) ArtifactDir() string { return t.dir }

func TestRequireNetworkIntegration(t *testing.T) {
	t.Run("skips in codex seatbelt sandbox", func(t *testing.T) {
		t.Setenv(codexSandboxEnv, codexSandboxSeatbelt)

		var skipped bool
		var continued bool

		ok := t.Run("subject", func(t *testing.T) {
			t.Cleanup(func() {
				skipped = t.Skipped()
			})

			RequireNetworkIntegration(t)
			continued = true
		})

		require.True(t, ok)
		require.True(t, skipped)
		require.False(t, continued)
	})

	t.Run("does not skip outside sandbox", func(t *testing.T) {
		t.Setenv(codexSandboxEnv, "")

		var skipped bool
		var continued bool

		ok := t.Run("subject", func(t *testing.T) {
			t.Cleanup(func() {
				skipped = t.Skipped()
			})

			RequireNetworkIntegration(t)
			continued = true
		})

		require.True(t, ok)
		require.False(t, skipped)
		require.True(t, continued)
	})

	t.Run("adds integration attr when supported", func(t *testing.T) {
		subject := &attrRecorderTB{TB: t}

		RequireNetworkIntegration(subject)

		require.Equal(t, "network", subject.attrs["integration"])
	})
}

func TestWriteArtifact(t *testing.T) {
	t.Run("writes artifact when supported", func(t *testing.T) {
		dir := t.TempDir()
		subject := artifactTB{TB: t, dir: dir}

		WriteArtifact(subject, "sample.txt", []byte("payload"))

		body, err := os.ReadFile(filepath.Join(dir, "sample.txt"))
		require.NoError(t, err)
		require.Equal(t, "payload", string(body))
	})

	t.Run("logs and returns on write failure", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "missing", "nested")
		subject := artifactTB{TB: t, dir: dir}

		WriteArtifact(subject, "sample.txt", []byte("payload"))

		_, err := os.Stat(filepath.Join(dir, "sample.txt"))
		require.Error(t, err)
		require.True(t, os.IsNotExist(err))
	})
}
