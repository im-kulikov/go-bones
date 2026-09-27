package testutil

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
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

func TestFreeTCPAddr(t *testing.T) {
	addr := FreeTCPAddr(t)

	host, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", host)
	require.NotEmpty(t, port)

	// The address must be immediately reusable: FreeTCPAddr already closed
	// its own probe listener before returning it.
	lis, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	require.NoError(t, lis.Close())
}

// failCloseListener wraps a real listener but reports a failure from Close,
// while still actually releasing the underlying port so tests don't leak it.
type failCloseListener struct {
	net.Listener
}

func (f failCloseListener) Close() error {
	_ = f.Listener.Close()

	return errors.New("close failed")
}

// fatalRecorderTB fakes just enough of testing.TB to observe a Fatalf call
// without it failing *this* test: a real t.Run subtest would propagate its
// failure to the parent regardless of what the parent asserts afterward,
// which is exactly what these tests need to avoid since a Fatalf call here
// is the expected, correct behavior being verified.
type fatalRecorderTB struct {
	testing.TB
	fataled bool
}

func (f *fatalRecorderTB) Helper() {}
func (f *fatalRecorderTB) Fatalf(string, ...any) {
	f.fataled = true
	runtime.Goexit()
}

func TestFreeTCPAddr_ListenFailure(t *testing.T) {
	prev := listenTCP
	t.Cleanup(func() { listenTCP = prev })

	listenTCP = func(context.Context) (net.Listener, error) {
		return nil, errors.New("listen failed")
	}

	fake := &fatalRecorderTB{TB: t}

	done := make(chan struct{})
	go func() {
		defer close(done)
		FreeTCPAddr(fake)
	}()
	<-done

	require.True(t, fake.fataled, "FreeTCPAddr must call Fatalf when Listen fails")
}

func TestFreeTCPAddr_CloseFailure(t *testing.T) {
	prev := listenTCP
	t.Cleanup(func() { listenTCP = prev })

	listenTCP = func(ctx context.Context) (net.Listener, error) {
		lis, err := new(net.ListenConfig).Listen(ctx, "tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}

		return failCloseListener{Listener: lis}, nil
	}

	fake := &fatalRecorderTB{TB: t}

	done := make(chan struct{})
	go func() {
		defer close(done)
		FreeTCPAddr(fake)
	}()
	<-done

	require.True(t, fake.fataled, "FreeTCPAddr must call Fatalf when Close fails")
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
