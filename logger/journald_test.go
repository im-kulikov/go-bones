package logger

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/im-kulikov/go-bones/config"
	"github.com/stretchr/testify/require"
)

// journalTestSocket listens on a socket of its own and points the handler at
// it, so that the tests never touch the journal of the machine they run on.
func journalTestSocket(t *testing.T) *net.UnixConn {
	t.Helper()

	path := filepath.Join(t.TempDir(), "journal.sock")

	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	require.NoError(t, err)

	t.Cleanup(func() { _ = conn.Close() })

	previous := journalSocket
	journalSocket = path

	t.Cleanup(func() { journalSocket = previous })

	return conn
}

func readJournal(t *testing.T, conn *net.UnixConn) string {
	t.Helper()

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))

	buf := make([]byte, 4096)

	n, _, err := conn.ReadFromUnix(buf)
	require.NoError(t, err)

	return string(buf[:n])
}

// Test_JournaldFields checks the native protocol: the attributes become fields
// of the entry rather than pieces of the message.
func Test_JournaldFields(t *testing.T) {
	conn := journalTestSocket(t)

	var out bytes.Buffer

	log := New(config.Logger{Format: "journald"}, NewJournalHandler(&out, nil))
	log.Warn("could not sync", "peer", "10.0.0.11:10102", "rows", 3)

	got := readJournal(t, conn)

	require.Contains(t, got, "PRIORITY=4\n")
	require.Contains(t, got, "MESSAGE=could not sync peer=10.0.0.11:10102 rows=3\n")
	require.Contains(t, got, "PEER=10.0.0.11:10102\n")
	require.Contains(t, got, "ROWS=3\n")
	require.NotContains(t, got, "msg=")
	require.NotContains(t, got, "level=")
	require.Empty(t, out.String())
}

// Test_JournaldWithoutSocket checks the fallback: with no journal to write to,
// the record still reaches the stream with the severity prefix the journal
// reads from it.
func Test_JournaldWithoutSocket(t *testing.T) {
	previous := journalSocket
	journalSocket = filepath.Join(t.TempDir(), "absent")

	t.Cleanup(func() { journalSocket = previous })

	for _, tt := range []struct {
		level slog.Level
		want  string
	}{
		{slog.LevelInfo, "<6>"},
		{slog.LevelWarn, "<4>"},
		{slog.LevelError, "<3>"},
	} {
		var out bytes.Buffer

		log := New(config.Logger{Format: "journald"}, NewJournalHandler(&out, nil))
		log.Log(context.Background(), tt.level, "authentication success", "user", "tsm")

		require.True(t, strings.HasPrefix(out.String(), tt.want),
			"line %q does not start with %q", out.String(), tt.want)
		require.NotContains(t, out.String(), "level=")
	}
}

// Test_JournaldEscapesNewlines checks a value carrying a newline: the journal
// separates fields with them, and a raw one breaks the whole datagram, which
// is how a service that refused to start logs no reason at all.
func Test_JournaldEscapesNewlines(t *testing.T) {
	conn := journalTestSocket(t)

	var out bytes.Buffer

	log := New(config.Logger{Format: "journald"}, NewJournalHandler(&out, nil))
	log.Error("could not start", "error", "listen tcp 127.0.0.1:10099\nbind: permission denied")

	got := readJournal(t, conn)

	// The value stays within one line of the datagram: a raw newline would
	// make the journal read the rest as a field of its own.
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	require.Contains(t, lines, `ERROR=listen tcp 127.0.0.1:10099\nbind: permission denied`)
}

// Test_JournaldFieldNames checks the names the journal accepts: uppercase only,
// and nothing that looks like a field of its own.
func Test_JournaldFieldNames(t *testing.T) {
	conn := journalTestSocket(t)

	var out bytes.Buffer

	log := New(config.Logger{Format: "journald"}, NewJournalHandler(&out, nil))
	log.Info("names", "peer", "a", "nas-ip", "b", "_hidden", "c", "9lives", "d")

	got := readJournal(t, conn)

	require.Contains(t, got, "PEER=a\n")
	require.Contains(t, got, "NAS_IP=b\n")
	require.Contains(t, got, "FIELD__HIDDEN=c\n")
	require.Contains(t, got, "FIELD_9LIVES=d\n")
}
