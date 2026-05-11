package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	codexSandboxEnv      = "CODEX_SANDBOX"
	codexSandboxSeatbelt = "seatbelt"
)

// RequireNetworkIntegration marks the test as a network integration scenario
// and skips it unless the dedicated env flag is enabled.
func RequireNetworkIntegration(t testing.TB) {
	t.Helper()
	t.Attr("integration", "network")

	if os.Getenv(codexSandboxEnv) == codexSandboxSeatbelt {
		t.Skip("network integration test is skipped in the Codex seatbelt sandbox; " +
			"rerun it outside the sandbox with escalated permissions")
	}
}

// WriteArtifact stores a test artifact when the current Go version exposes ArtifactDir.
func WriteArtifact(t testing.TB, name string, content []byte) {
	t.Helper()
	path := filepath.Join(t.ArtifactDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Logf("write artifact %s: %v", name, err)
		return
	}

	t.Logf("artifact: %s", path)
}
