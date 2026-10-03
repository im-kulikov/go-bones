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
	mu      *sync.Mutex
	out     io.Writer
	buf     *bytes.Buffer
	attrs   slog.Handler
	replace func([]string, slog.Attr) slog.Attr
	color   bool
}

// NewConsoleHandler returns the handler behind LOGGER_FORMAT=console: colored,
// human-readable lines for local development. Prefer text or json in
// production, where logs are parsed by machines.
//
// As in the slog handlers, opts.ReplaceAttr also gets the time, level and
// message: a changed value is printed in its place, an empty attribute drops
// it from the line. Their keys are not printed.
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
		mu:      new(sync.Mutex),
		out:     w,
		buf:     buf,
		attrs:   slog.NewTextHandler(buf, &o),
		replace: replace,
		color:   os.Getenv("NO_COLOR") == "",
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

	parts := make([]string, 0, 4)
	if !r.Time.IsZero() { // a zero time is omitted before ReplaceAttr, as in slog
		if v, ok := h.builtin(slog.Time(slog.TimeKey, r.Time)); ok {
			parts = append(parts, h.paint(colorDim, consoleTime(v)))
		}
	}

	if v, ok := h.builtin(slog.Any(slog.LevelKey, r.Level)); ok {
		parts = append(parts, h.level(v, r.Level))
	}

	if v, ok := h.builtin(slog.String(slog.MessageKey, r.Message)); ok {
		parts = append(parts, oneLine(v.String()))
	}

	if attrs := strings.TrimSuffix(h.buf.String(), "\n"); attrs != "" {
		parts = append(parts, attrs)
	}

	_, err := io.WriteString(h.out, strings.Join(parts, " ")+"\n")

	return err
}

// builtin passes a built-in attribute through ReplaceAttr, as the slog
// handlers do, and reports whether it is still printed.
func (h *consoleHandler) builtin(a slog.Attr) (slog.Value, bool) {
	if h.replace != nil {
		a = h.replace(nil, a)
	}

	return a.Value.Resolve(), !a.Equal(slog.Attr{})
}

// oneLine escapes line breaks, so a message cannot add lines to the log.
func oneLine(s string) string {
	if !strings.ContainsAny(s, "\r\n") {
		return s
	}

	return strings.NewReplacer("\n", `\n`, "\r", `\r`).Replace(s)
}

func consoleTime(v slog.Value) string {
	if v.Kind() == slog.KindTime {
		return v.Time().Format("15:04:05.000")
	}

	return v.String()
}

// level prints a level as three colored letters; a value ReplaceAttr made of
// it that is not a Level is printed as is, in the color of the record level.
func (h *consoleHandler) level(v slog.Value, l Level) string {
	if replaced, ok := v.Any().(Level); ok {
		return h.paint(levelStyle(replaced))
	}

	color, _ := levelStyle(l)

	return h.paint(color, v.String())
}

func levelStyle(l Level) (color, name string) {
	switch {
	case l < slog.LevelInfo:
		return colorBlue, "DBG"
	case l < slog.LevelWarn:
		return colorGreen, "INF"
	case l < slog.LevelError:
		return colorYelow, "WRN"
	default:
		return colorRed, "ERR"
	}
}

func (h *consoleHandler) paint(color, s string) string {
	if !h.color {
		return s
	}

	return color + s + colorReset
}
