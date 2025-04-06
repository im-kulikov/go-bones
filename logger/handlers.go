package logger

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/im-kulikov/go-bones/config"
)

type (
	// NamedLogger allows to set named prefix for Logger message.
	NamedLogger interface {
		Named(name string) Handler
	}

	slogTransformer interface {
		Transform(ctx context.Context, record Record) Record
	}

	slogTransformerFunc func(ctx context.Context, record Record) Record

	wrappedHandler struct {
		name []string
		next Handler
		conf config.Logger
		list []slogTransformer
	}
)

// Transform Record by slogTransformerFunc.
func (s slogTransformerFunc) Transform(ctx context.Context, record Record) Record {
	return s(ctx, record)
}

// Enabled returns bool that identifies is Level enabled.
func (h *wrappedHandler) Enabled(ctx context.Context, level Level) bool {
	return h.next.Enabled(ctx, level)
}

// WithAttrs returns a new Handler whose attributes consist of
// both the receiver's attributes and the arguments.
// The Handler owns the slice: it may retain, modify or discard it.
func (h *wrappedHandler) WithAttrs(attrs []Attr) Handler {
	return &wrappedHandler{
		conf: h.conf,
		next: h.next.WithAttrs(attrs),
		list: slices.Clone(h.list),
	}
}

// WithGroup returns a new Handler with the given group appended to
// the receiver's existing groups.
func (h *wrappedHandler) WithGroup(name string) Handler {
	return &wrappedHandler{
		conf: h.conf,
		next: h.next.WithGroup(name),
		list: slices.Clone(h.list),
	}
}

// Handle handles the Record.
func (h *wrappedHandler) Handle(ctx context.Context, original Record) error {
	redacted := original
	for _, handler := range h.list {
		redacted = handler.Transform(ctx, redacted)
	}

	return h.next.Handle(ctx, h.applyNames(redacted))
}

// Named allows to set message prefix if Handler implements NamedLogger.
func Named(log *Logger, name string) *Logger {
	if handler, ok := log.Handler().(NamedLogger); ok {
		return newLogger(handler.Named(name))
	}

	return log
}

// Named produces new Handler wrapped with message prefix.
func (h *wrappedHandler) Named(name string) Handler {
	return &wrappedHandler{
		conf: h.conf,
		next: h.next,
		list: h.list,
		name: append(h.name, name),
	}
}

func (h *wrappedHandler) applyNames(redacted Record) slog.Record {
	if len(h.name) == 0 {
		return redacted
	}

	redacted.Message = fmt.Sprintf("[%s] %s",
		strings.Join(h.name, ":"), redacted.Message)

	return redacted
}
