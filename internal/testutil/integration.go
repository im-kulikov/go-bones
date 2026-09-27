package testutil

import (
	"context"
	"net"
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

// listenTCP is a seam over net.ListenConfig.Listen so tests in this package
// can simulate Listen/Close failures without needing to actually exhaust OS
// resources (file descriptors, ephemeral ports).
//
//nolint:gochecknoglobals
var listenTCP = func(ctx context.Context) (net.Listener, error) {
	return new(net.ListenConfig).Listen(ctx, "tcp", "127.0.0.1:0")
}

// FreeTCPAddr returns a loopback TCP address that was free at the time of the
// call, for tests that need a real address to configure a server with before
// starting it.
//
// This is a listen-then-close-then-reuse trick, which is inherently prone to
// a TOCTOU race: something else on the machine could bind the same port
// between this call returning and the caller's own Listen. That risk is
// accepted here - centralizing the pattern at least keeps it in one place
// rather than duplicated across every integration test.
func FreeTCPAddr(t testing.TB) string {
	t.Helper()

	lis, err := listenTCP(context.Background())
	if err != nil {
		t.Fatalf("find free tcp addr: %v", err)
	}

	addr := lis.Addr().String()
	if err = lis.Close(); err != nil {
		t.Fatalf("close free tcp addr probe listener: %v", err)
	}

	return addr
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
