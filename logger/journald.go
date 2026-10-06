package logger

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// journalSocket is where journald listens for its native protocol: one datagram
// becomes one entry, and every KEY=value line of that datagram becomes a field
// of the entry. journalctl can then select on them, and -o json carries them.
//
// It is a variable so that tests can point the handler at a socket of their own.
//
//nolint:gochecknoglobals
var journalSocket = "/run/systemd/journal/socket"

// NewJournalHandler builds a handler that writes to journald through the native
// protocol: the message stays the message of the entry, and every attribute
// becomes a field of it, so that `journalctl PEER=...` and `journalctl -o json`
// see them.
//
// Where the socket is not there to write to — a container without journald, a
// host without systemd — the record falls back to the plain line with an
// RFC 5424 severity prefix, which is what journald reads from a stream.
func NewJournalHandler(w io.Writer, o *HandlerOptions) Handler {
	level := slog.LevelInfo
	if o != nil && o.Level != nil {
		level = o.Level.Level()
	}

	opts := &slog.HandlerOptions{Level: level}
	if o != nil {
		opts.AddSource = o.AddSource
	}

	// The journal keeps the time and the level of an entry itself, and the
	// message is written in front of the attributes rather than under a key.
	drop := func(groups []string, attr slog.Attr) slog.Attr {
		if len(groups) > 0 {
			return attr
		}

		switch attr.Key {
		case slog.TimeKey, slog.LevelKey, slog.MessageKey:
			return slog.Attr{}
		}

		return attr
	}

	opts.ReplaceAttr = drop
	if o != nil && o.ReplaceAttr != nil {
		keep := o.ReplaceAttr
		opts.ReplaceAttr = func(groups []string, attr slog.Attr) slog.Attr {
			if attr = drop(groups, attr); attr.Equal(slog.Attr{}) {
				return attr
			}

			return keep(groups, attr)
		}
	}

	return &journalHandler{out: w, opts: opts, level: level}
}

// journalHandler writes records to journald, and to a plain stream where the
// journal is not there.
type journalHandler struct {
	out   io.Writer
	opts  *slog.HandlerOptions
	level slog.Level
	attrs []slog.Attr
	group string
}

// Enabled implements slog.Handler.
func (h *journalHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle implements slog.Handler.
func (h *journalHandler) Handle(ctx context.Context, record slog.Record) error {
	var buf bytes.Buffer

	// What the text handler renders here are the attributes alone: the message
	// and the level were taken out, so the line opens with the text an operator
	// reads rather than with the msg= key.
	inner := slog.NewTextHandler(&buf, h.opts).WithAttrs(h.attrs)
	if h.group != "" {
		inner = inner.WithGroup(h.group)
	}

	if err := inner.Handle(ctx, record); err != nil {
		return err
	}

	line := strings.TrimRight(record.Message+" "+buf.String(), " \n")
	priority := severity(record.Level)

	// The native protocol is tried per record: journald may be gone, and the
	// line with the severity prefix is what a stream still gets across.
	if err := sendToJournal(priority, line, h.attrs, record); err == nil {
		return nil
	}

	_, err := fmt.Fprintf(h.out, "<%d>%s\n", priority, line)

	return err
}

// WithAttrs implements slog.Handler.
func (h *journalHandler) WithAttrs(attrs []slog.Attr) Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)

	return &clone
}

// WithGroup implements slog.Handler.
func (h *journalHandler) WithGroup(name string) Handler {
	clone := *h
	clone.group = name

	return &clone
}

// sendToJournal hands one record to journald over its native protocol, where
// the message and every attribute become fields of the entry.
//
// ponytail: the socket is opened per record; keep a connection if the log rate
// ever makes the syscall matter.
func sendToJournal(priority int, message string, attrs []slog.Attr, record slog.Record) error {
	conn, err := net.Dial("unixgram", journalSocket)
	if err != nil {
		return err
	}

	defer conn.Close() //nolint:errcheck // nothing to do about a failed close

	var buf bytes.Buffer

	fmt.Fprintf(&buf, "PRIORITY=%d\n", priority)
	fmt.Fprintf(&buf, "SYSLOG_IDENTIFIER=%s\n", journalIdentifier())
	writeField(&buf, "MESSAGE", message)

	for _, attr := range attrs {
		writeAttr(&buf, attr)
	}

	record.Attrs(func(attr slog.Attr) bool {
		writeAttr(&buf, attr)

		return true
	})

	_, err = conn.Write(buf.Bytes())

	return err
}

// journalIdentifier is the name the records are filed under: journalctl -t
// takes it, and it is the name the service was started as.
func journalIdentifier() string {
	if len(os.Args) == 0 {
		return ""
	}

	return filepath.Base(os.Args[0])
}

// writeAttr adds one attribute as a field, keeping the names of the groups it
// sits in.
func writeAttr(buf *bytes.Buffer, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()

	if attr.Key == "" {
		return
	}

	if attr.Value.Kind() == slog.KindGroup {
		for _, nested := range attr.Value.Group() {
			nested.Key = attr.Key + "_" + nested.Key

			writeAttr(buf, nested)
		}

		return
	}

	writeField(buf, fieldName(attr.Key), attr.Value.String())
}

// writeField adds one field to the datagram.
//
// The journal separates fields with newlines, so a value carrying one belongs
// in the size-prefixed form: the name, a newline, the length as a little-endian
// uint64, the value, a newline. That form keeps an error message multi-line —
// and a multi-line message breaks every reader that walks the journal line by
// line. The control characters are spelled out instead, the way the text
// handler spells them: the record stays one line, and still says what happened.
func writeField(buf *bytes.Buffer, name, value string) {
	fmt.Fprintf(buf, "%s=%s\n", name, escapeValue(value))
}

// escapeValue spells the characters the journal would take for field
// separators.
func escapeValue(value string) string {
	if !strings.ContainsAny(value, "\n\r") {
		return value
	}

	return strings.NewReplacer("\n", `\n`, "\r", `\r`).Replace(value)
}

// fieldName turns an attribute key into a field name. The journal takes
// uppercase letters, digits and underscores and ignores everything else —
// silently, a lower-case field never reaches the entry at all.
func fieldName(key string) string {
	var b strings.Builder

	for _, r := range strings.ToUpper(key) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}

	name := b.String()

	// Names starting with an underscore or a digit belong to the journal
	// itself, so a field of ours must not look like one.
	if name == "" || name[0] == '_' || (name[0] >= '0' && name[0] <= '9') {
		return "FIELD_" + name
	}

	return name
}

// severity maps a level to the syslog priority of RFC 5424: 7 debug, 6 info,
// 4 warning, 3 error.
func severity(level slog.Level) int {
	switch {
	case level < slog.LevelInfo:
		return 7
	case level < slog.LevelWarn:
		return 6
	case level < slog.LevelError:
		return 4
	default:
		return 3
	}
}
