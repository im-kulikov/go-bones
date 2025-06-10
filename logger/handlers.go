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
	// NamedLogger represents a handler that can set a name-based prefix for log messages.
	NamedLogger interface {
		// Named creates a new handler with a message prefix based on the provided name.
		Named(name string) Handler
	}

	// slogTransformer defines an interface for transforming log records.
	// Custom implementations can modify log records before they are handled.
	slogTransformer interface {
		// Transform modifies the provided log record within the given context.
		//
		// Parameters:
		//   - ctx: The context for the logging operation.
		//   - record: The original log record to transform.
		//
		// Returns:
		//   - A transformed log record.
		Transform(ctx context.Context, record Record) Record
	}

	// slogTransformerFunc is a function adapter for the slogTransformer interface.
	slogTransformerFunc func(ctx context.Context, record Record) Record

	// wrappedHandler is a custom handler that wraps another handler and provides
	// features such as name-based prefixes and record transformations.
	wrappedHandler struct {
		// name holds a hierarchy of prefix names applied to log messages.
		name []string

		// next is the underlying handler responsible for processing the log records.
		next Handler

		// conf represents the logger configuration.
		conf config.Logger

		// list is a set of slogTransformer implementations used to modify log records.
		list []slogTransformer
	}
)

// Transform applies the slogTransformerFunc to transform log records.
//
// Parameters:
//   - ctx: The context for the logging operation.
//   - record: The original log record to transform.
//
// Returns:
//   - A transformed log record.
func (s slogTransformerFunc) Transform(ctx context.Context, record Record) Record {
	return s(ctx, record)
}

// Enabled determines if the wrapped handler is enabled for the specified log level.
//
// Parameters:
//   - ctx: The logging context.
//   - level: The log level to check.
//
// Returns:
//   - `true` if the level is enabled; `false` otherwise.
func (h *wrappedHandler) Enabled(ctx context.Context, level Level) bool {
	return h.next.Enabled(ctx, level)
}

// WithAttrs returns a new wrapped handler with the specified attributes.
// These attributes are combined with the receiver's existing attributes.
//
// Parameters:
//   - attrs: A slice of attributes to add.
//
// Returns:
//   - A new handler with the combined attributes.
func (h *wrappedHandler) WithAttrs(attrs []Attr) Handler {
	return &wrappedHandler{
		conf: h.conf,
		next: h.next.WithAttrs(attrs),
		list: slices.Clone(h.list),
	}
}

// WithGroup returns a new wrapped handler appending the specified group name
// to the receiver's group hierarchy.
//
// Parameters:
//   - name: The name of the group to append.
//
// Returns:
//   - A new handler with the updated group hierarchy.
func (h *wrappedHandler) WithGroup(name string) Handler {
	return &wrappedHandler{
		conf: h.conf,
		next: h.next.WithGroup(name),
		list: slices.Clone(h.list),
	}
}

// Handle applies all record transformations and passes the modified log record
// to the underlying handler for processing.
//
// Parameters:
//   - ctx: The logging context.
//   - original: The original log record to process.
//
// Returns:
//   - An error if the log record processing fails; otherwise, nil.
func (h *wrappedHandler) Handle(ctx context.Context, original Record) error {
	redacted := original
	for _, handler := range h.list {
		redacted = handler.Transform(ctx, redacted)
	}

	return h.next.Handle(ctx, h.applyNames(redacted))
}

// Named assigns a name-based prefix to the log messages if the handler supports NamedLogger.
//
// Parameters:
//   - log: The logger for which the prefix is to be set.
//   - name: The name to use as the prefix.
//
// Returns:
//   - A new Logger instance with the prefix applied.
func Named(log *Logger, name string) *Logger {
	if handler, ok := log.Handler().(NamedLogger); ok {
		return newLogger(handler.Named(name))
	}

	return log
}

// Named appends the specified name to the list of prefixes and returns a new handler.
//
// Parameters:
//   - name: The name to append.
//
// Returns:
//   - A new handler with the updated name prefix list.
func (h *wrappedHandler) Named(name string) Handler {
	return &wrappedHandler{
		conf: h.conf,
		next: h.next,
		list: h.list,
		name: append(h.name, name),
	}
}

// applyNames adds the name-based prefix to the log record's message if any names are set.
//
// Parameters:
//   - redacted: The log record to apply the prefix.
//
// Returns:
//   - The updated log record with the prefix applied.
func (h *wrappedHandler) applyNames(redacted Record) slog.Record {
	if len(h.name) == 0 {
		return redacted
	}

	redacted.Message = fmt.Sprintf("[%s] %s",
		strings.Join(h.name, ":"), redacted.Message)

	return redacted
}
