package logger

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/config"
)

type failingHandler struct{ Handler }

func (failingHandler) Handle(context.Context, Record) error { return errors.New("write failed") }

func TestConsoleHandler(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	buf := new(bytes.Buffer)
	h := NewConsoleHandler(buf, &HandlerOptions{Level: slog.LevelDebug})

	at := time.Date(2026, 9, 27, 21, 15, 3, 120_000_000, time.UTC)
	for _, level := range []Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		require.NoError(t, h.Handle(t.Context(), slog.NewRecord(at, level, "hello", 0)))
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Equal(
		t,
		colorDim+"21:15:03.120"+colorReset+" "+colorBlue+"DBG"+colorReset+" hello",
		lines[0],
	)
	require.Contains(t, lines[1], colorGreen+"INF"+colorReset)
	require.Contains(t, lines[2], colorYelow+"WRN"+colorReset)
	require.Contains(t, lines[3], colorRed+"ERR"+colorReset)
}

func TestConsoleHandler_AttrsGroupsAndOptions(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	buf := new(bytes.Buffer)
	h := NewConsoleHandler(buf, &HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == "secret" {
				a.Value = slog.StringValue("***")
			}

			return a
		},
	})

	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "request handled", 0)
	r.AddAttrs(slog.Int("status", 200), slog.String("secret", "x"))
	require.NoError(
		t,
		h.WithAttrs([]Attr{slog.String("service", "api")}).WithGroup("http").Handle(t.Context(), r),
	)

	require.Equal(
		t,
		"INF request handled service=api http.status=200 http.secret=***\n",
		buf.String(),
		"zero time is skipped, attributes are formatted like the text format",
	)

	buf.Reset()
	require.False(t, h.Enabled(t.Context(), slog.LevelDebug))
	require.NoError(
		t,
		NewConsoleHandler(
			buf,
			nil,
		).Handle(t.Context(), slog.NewRecord(time.Time{}, slog.LevelWarn, "bare", 0)),
	)
	require.Equal(t, "WRN bare\n", buf.String())

	broken := &consoleHandler{
		mu:    h.(*consoleHandler).mu,
		buf:   new(bytes.Buffer),
		attrs: failingHandler{},
	}
	require.EqualError(t, broken.Handle(t.Context(), r), "write failed")
}

func TestInit_ConsoleFormat(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	prev := Default()
	t.Cleanup(func() { defaultLogger.Store(prev) })

	buf := new(bytes.Buffer)
	log := Init(config.Logger{Format: "console", Level: "info"}, WithOutput(buf))
	log.Info("started", String("version", "dev"))

	require.Regexp(t, `^\d\d:\d\d:\d\d\.\d{3} INF started version=dev\n$`, buf.String())
}

func TestFormatFor(t *testing.T) {
	buf := new(bytes.Buffer)
	record := slog.NewRecord(time.Time{}, slog.LevelInfo, "hello", 0)

	for format, want := range map[string]string{
		"":        `{"level":"INFO","msg":"hello"}` + "\n",
		"json":    `{"level":"INFO","msg":"hello"}` + "\n",
		"text":    "level=INFO msg=hello\n",
		"console": "INF hello\n",
	} {
		t.Setenv("NO_COLOR", "1")
		buf.Reset()
		require.NoError(t, formatFor(format)(buf, nil).Handle(t.Context(), record), format)
		require.Equal(t, want, buf.String(), format)
	}
}

// TestConsoleHandler_ReplaceAttrBuiltins: like the slog handlers, ReplaceAttr
// also gets time, level and msg; a changed value is printed in its place and
// an empty attribute drops it.
func TestConsoleHandler_ReplaceAttrBuiltins(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	at := time.Date(2026, 9, 27, 21, 15, 3, 0, time.UTC)

	t.Run("replaced", func(t *testing.T) {
		buf := new(bytes.Buffer)
		h := NewConsoleHandler(buf, &HandlerOptions{
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if len(groups) > 0 {
					return a
				}

				switch a.Key {
				case slog.TimeKey:
					return slog.String(a.Key, "now")
				case slog.LevelKey:
					if a.Value.Any() == slog.LevelWarn {
						return slog.String(a.Key, "WARNING")
					}

					return slog.Any(a.Key, slog.LevelError)
				case slog.MessageKey:
					return slog.String(a.Key, strings.ToUpper(a.Value.String()))
				}

				return a
			},
		})

		require.NoError(t, h.Handle(t.Context(), slog.NewRecord(at, slog.LevelInfo, "hello", 0)))
		require.NoError(t, h.Handle(t.Context(), slog.NewRecord(at, slog.LevelWarn, "careful", 0)))
		require.Equal(t, "now ERR HELLO\nnow WARNING CAREFUL\n", buf.String())
	})

	t.Run("dropped", func(t *testing.T) {
		buf := new(bytes.Buffer)
		h := NewConsoleHandler(buf, &HandlerOptions{
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				switch {
				case len(groups) > 0:
					return a
				case a.Key == slog.TimeKey, a.Key == slog.LevelKey, a.Key == slog.MessageKey:
					return slog.Attr{}
				}

				return a
			},
		})

		r := slog.NewRecord(at, slog.LevelInfo, "hello", 0)
		r.AddAttrs(slog.Int("status", 200))
		require.NoError(t, h.Handle(t.Context(), r))
		require.Equal(t, "status=200\n", buf.String())
	})
}

func TestConsoleHandler_ZeroTimeSkipsReplaceAttr(t *testing.T) {
	var keys []string

	h := NewConsoleHandler(new(bytes.Buffer), &HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			keys = append(keys, a.Key)

			return a
		},
	})

	require.NoError(t, h.Handle(t.Context(), slog.NewRecord(time.Time{}, slog.LevelInfo, "hi", 0)))
	require.Equal(t, []string{slog.LevelKey, slog.MessageKey}, keys)
}
