package logger

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// ANSI colors of the console format.
const (
	colorReset = "\x1b[0m"
	colorDim   = "\x1b[90m"
	colorRed   = "\x1b[31m"
	colorGreen = "\x1b[32m"
	colorYelow = "\x1b[33m"
	colorBlue  = "\x1b[34m"
)

// consoleHandler writes one human-readable line per record for development:
//
//	15:04:05.000 INF [api] request handled method=GET status=200
//
// Time, level and message come first, colored unless NO_COLOR is set; the
// attributes, groups and source are formatted by a slog.TextHandler, so they
// look exactly like the text format.
type consoleHandler struct {
	mu    *sync.Mutex
	out   io.Writer
	buf   *bytes.Buffer
	attrs slog.Handler
	color bool
}

// NewConsoleHandler returns the handler behind LOGGER_FORMAT=console: colored,
// human-readable lines for local development. Prefer text or json in
// production, where logs are parsed by machines.
func NewConsoleHandler(w io.Writer, opts *HandlerOptions) Handler {
	var o HandlerOptions
	if opts != nil {
		o = *opts
	}

	replace := o.ReplaceAttr
	o.ReplaceAttr = func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) == 0 {
			switch a.Key {
			case slog.TimeKey, slog.LevelKey, slog.MessageKey:
				return slog.Attr{} // printed by the console prefix
			}
		}

		if replace != nil {
			return replace(groups, a)
		}

		return a
	}

	buf := new(bytes.Buffer)

	return &consoleHandler{
		mu:    new(sync.Mutex),
		out:   w,
		buf:   buf,
		attrs: slog.NewTextHandler(buf, &o),
		color: os.Getenv("NO_COLOR") == "",
	}
}

func (h *consoleHandler) Enabled(ctx context.Context, level Level) bool {
	return h.attrs.Enabled(ctx, level)
}

func (h *consoleHandler) WithAttrs(attrs []Attr) Handler {
	clone := *h
	clone.attrs = h.attrs.WithAttrs(attrs)

	return &clone
}

func (h *consoleHandler) WithGroup(name string) Handler {
	clone := *h
	clone.attrs = h.attrs.WithGroup(name)

	return &clone
}

func (h *consoleHandler) Handle(ctx context.Context, r Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.buf.Reset()
	if err := h.attrs.Handle(ctx, r); err != nil {
		return err
	}

	var line strings.Builder
	if !r.Time.IsZero() {
		line.WriteString(h.paint(colorDim, r.Time.Format("15:04:05.000")))
		line.WriteByte(' ')
	}

	line.WriteString(h.level(r.Level))
	line.WriteByte(' ')
	line.WriteString(r.Message)

	if attrs := strings.TrimSuffix(h.buf.String(), "\n"); attrs != "" {
		line.WriteByte(' ')
		line.WriteString(attrs)
	}

	line.WriteByte('\n')

	_, err := io.WriteString(h.out, line.String())

	return err
}

func (h *consoleHandler) level(l Level) string {
	switch {
	case l < slog.LevelInfo:
		return h.paint(colorBlue, "DBG")
	case l < slog.LevelWarn:
		return h.paint(colorGreen, "INF")
	case l < slog.LevelError:
		return h.paint(colorYelow, "WRN")
	default:
		return h.paint(colorRed, "ERR")
	}
}

func (h *consoleHandler) paint(color, s string) string {
	if !h.color {
		return s
	}

	return color + s + colorReset
}
