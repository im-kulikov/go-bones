package logger

import (
	"context"
	"io"
	"iter"
	"log/slog"
	"sync/atomic"

	"github.com/im-kulikov/go-bones/config"
)

// defaultLogger is a globally accessible logger instance that can be shared across the application.
// nolint:gochecknoglobals
var defaultLogger atomic.Pointer[Logger]

func init() { defaultLogger.Store(slog.Default()) }

// optionsFromConfig converts a config.Logger and a slice of additional Option
// functions into an iter.Seq of Option functions. It applies configuration
// values for AddSource, Level, and Format, falling back to defaults and logging
// warnings if parsing fails.
//
// Supported formats:
//   - `json`: JSON-formatted logs.
//   - `text`: human-readable text logs (default).
//
// Supported levels (see slog.Level): DEBUG, INFO, WARN, ERROR
//
// Any extra Option functions in a slice of Option are appended after config-derived options.
func optionsFromConfig(cfg config.Logger, opts []Option) iter.Seq[Option] {
	return func(yield func(Option) bool) {
		if !yield(WithSource(cfg.AddSource)) {
			return
		}

		var lvl Level
		if err := lvl.UnmarshalText([]byte(cfg.Level)); err != nil {
			Warn("could not parse logger.level", String("level", cfg.Level), Err(err))
		} else if !yield(WithLevel(lvl)) {
			return
		}

		format := func(w io.Writer, o *HandlerOptions) Handler { return slog.NewTextHandler(w, o) }
		switch cfg.Format {
		case "json":
			format = func(w io.Writer, o *HandlerOptions) Handler { return slog.NewJSONHandler(w, o) }
		case "text":
			// already set
		default:
			Warn("could not parse logger.format", String("format", cfg.Format))
		}

		if !yield(func(o *options) { o.format = format }) {
			return
		}

		for _, option := range opts {
			if !yield(option) {
				return
			}
		}
	}
}

// Init initializes the default logger with the given configuration and options.
// The default logger is set globally, making it available for top-level functions
// such as Debug, Info, Warn, and Error.
//
// Parameters:
//   - cfg: Logger configuration.
//   - opts: Optional configuration options that can include:
//   - Custom handler via WithHandler
//   - Log transformers via WithTransformers
//   - Output destination via WithOutput
//   - Log level via WithLevel
//
// Returns:
//   - A pointer to the initialized Logger.
func Init(cfg config.Logger, opts ...Option) *Logger {
	var o options
	for option := range optionsFromConfig(cfg, opts) {
		option(&o)
	}

	o.setDefaults()

	logger := New(cfg, o.handler, o.transformers...)
	defaultLogger.Store(logger)

	return logger
}

// Debug logs a message with LevelDebug severity using the default logger.
// This is typically used for development and debugging.
//
// Parameters:
//   - msg: The message to log.
//   - attrs: Additional attributes to include in the log record.
func Debug(msg string, attrs ...Attr) {
	DebugContext(context.Background(), msg, attrs...)
}

// DebugContext logs a message with LevelDebug severity using the default logger.
// This is typically used for development and debugging.
//
// Parameters:
//   - ctx: The context containing additional metadata, such as deadlines or context-specific attributes.
//   - msg: The message to log.
//   - attrs: Additional attributes to include in the log record.
func DebugContext(ctx context.Context, msg string, attrs ...Attr) {
	defaultLogger.Load().LogAttrs(ctx, slog.LevelDebug, msg, attrs...)
}

// Info logs a message with LevelInfo severity using the default logger.
// This is typically used for general informational messages.
//
// Parameters:
//   - msg: The message to log.
//   - attrs: Additional attributes to include in the log record.
func Info(msg string, attrs ...Attr) {
	InfoContext(context.Background(), msg, attrs...)
}

// InfoContext logs a message with LevelInfo severity using the default logger.
// This is typically used for general informational messages.
//
// Parameters:
//   - ctx: The context containing additional metadata, such as deadlines or context-specific attributes.
//   - msg: The message to log.
//   - attrs: Additional attributes to include in the log record.
func InfoContext(ctx context.Context, msg string, attrs ...Attr) {
	defaultLogger.Load().LogAttrs(ctx, slog.LevelInfo, msg, attrs...)
}

// Warn logs a message with LevelWarn severity using the default logger.
// This is typically used to indicate something unexpected but not necessarily an error.
//
// Parameters:
//   - msg: The message to log.
//   - attrs: Additional attributes to include in the log record.
func Warn(msg string, attrs ...Attr) {
	WarnContext(context.Background(), msg, attrs...)
}

// WarnContext logs a message with LevelWarn severity using the default logger.
// This is typically used to indicate something unexpected but not necessarily an error.
//
// Parameters:
//   - ctx: The context containing additional metadata, such as deadlines or context-specific attributes.
//   - msg: The message to log.
//   - attrs: Additional attributes to include in the log record.
func WarnContext(ctx context.Context, msg string, attrs ...Attr) {
	defaultLogger.Load().LogAttrs(ctx, slog.LevelWarn, msg, attrs...)
}

// Error logs a message with LevelError severity using the default logger.
// This is typically used to record error events.
//
// Parameters:
//   - msg: The message to log.
//   - attrs: Additional attributes to include in the log record.
func Error(msg string, attrs ...Attr) {
	ErrorContext(context.Background(), msg, attrs...)
}

// ErrorContext logs a message with LevelError severity using the default logger.
// This is typically used to record error events.
//
// Parameters:
//   - ctx: The context containing additional metadata, such as deadlines or context-specific attributes.
//   - msg: The message to log.
//   - attrs: Additional attributes to include in the log record.
func ErrorContext(ctx context.Context, msg string, attrs ...Attr) {
	defaultLogger.Load().LogAttrs(ctx, slog.LevelError, msg, attrs...)
}
