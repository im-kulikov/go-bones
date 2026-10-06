package logger_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
)

// journal is the text format with the syslog priority that journald reads from
// the start of a stdout line (systemd StandardOutput=journal), so
// journalctl -p filters by level. journald stamps entries itself, so the time
// is left out.
type journal struct {
	slog.Handler // a TextHandler writing to out

	out *prefixWriter
}

// prefixWriter puts prefix before each write; a TextHandler writes a record at once.
type prefixWriter struct {
	mu     sync.Mutex
	w      io.Writer
	prefix string
}

func (p *prefixWriter) Write(b []byte) (int, error) {
	if _, err := io.WriteString(p.w, p.prefix+string(b)); err != nil {
		return 0, err
	}

	return len(b), nil
}

func newJournal(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
	out := &prefixWriter{w: w}

	return journal{slog.NewTextHandler(out, opts), out}
}

func (j journal) Handle(ctx context.Context, r slog.Record) error {
	j.out.mu.Lock()
	defer j.out.mu.Unlock()

	j.out.prefix = priority(r.Level)
	r.Time = time.Time{}

	return j.Handler.Handle(ctx, r)
}

func (j journal) WithAttrs(attrs []slog.Attr) slog.Handler {
	return journal{j.Handler.WithAttrs(attrs), j.out}
}

func (j journal) WithGroup(name string) slog.Handler {
	return journal{j.Handler.WithGroup(name), j.out}
}

// priority is the syslog severity of a level: <7> debug, <6> info, <4> warning, <3> error.
func priority(l slog.Level) string {
	switch {
	case l < slog.LevelInfo:
		return "<7>"
	case l < slog.LevelWarn:
		return "<6>"
	case l < slog.LevelError:
		return "<4>"
	default:
		return "<3>"
	}
}

// A journal format for services under systemd: register it in main, before
// app.Init, and set Environment=LOGGER_FORMAT=journal in the unit.
func ExampleRegisterFormat_journal() {
	logger.RegisterFormat("journal", newJournal)

	cfg := config.Logger{Format: "journal", Level: "info"} // LOGGER_FORMAT=journal
	log := logger.Init(cfg, logger.WithOutput(os.Stdout))
	log.With("peer", "10.0.0.1").Warn("login failed", "user", "ivanov")
	log.Error("radius unreachable")
	// Output:
	// <4>level=WARN msg="login failed" peer=10.0.0.1 user=ivanov
	// <3>level=ERROR msg="radius unreachable"
}
