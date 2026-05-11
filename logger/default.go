package logger

import (
	"context"
	"io"
	"iter"
	"log/slog"
	"sync/atomic"

	"github.com/im-kulikov/go-bones/config"
)

// defaultLogger stores the process-wide fallback logger used by package-level helpers.
//
// This global is an intentional design choice, not an accidental hidden dependency.
// The package keeps it for two reasons:
//   - Package-level helpers such as Info and Error stay ergonomic;
//   - Logging remains available before explicit logger initialization.
//
// The tradeoff is a shared mutable state. Callers that need strict isolation,
// deterministic tests, or independently configured logging should avoid the
// process-wide logger and use explicit instances created with New instead.
// nolint:gochecknoglobals
var defaultLogger atomic.Pointer[Logger]

func init() { defaultLogger.Store(slog.Default()) }

// Default returns the current process-wide logger used by package-level logging helpers.
// Prefer explicit logger instances when code should not depend on global process state.
func Default() *Logger {
	return defaultLogger.Load()
}

// optionsFromConfig translates config.Logger plus extra options into the option
// sequence consumed by Init.
//
// Config-derived options are emitted first so explicit opts can still append more
// behaviour. Invalid config values do not fail initialization; they fall back to
// defaults and emit warnings through the current default logger.
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

// Init rebuilds the process-wide default logger from config and stores it globally.
//
// This is the bridge between configuration loading and package-level logging helpers
// such as Info or Error. After Init, all top-level logging functions immediately use
// the newly constructed logger. Code that must avoid global state should use New
// directly and pass the resulting logger explicitly.
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

// Debug logs with LevelDebug using the current process-wide default logger.
func Debug(msg string, attrs ...Attr) {
	DebugContext(context.Background(), msg, attrs...)
}

// DebugContext logs with LevelDebug using the current process-wide default logger.
// Context is passed through so context-bound attrs and tracing metadata can be attached.
func DebugContext(ctx context.Context, msg string, attrs ...Attr) {
	defaultLogger.Load().LogAttrs(ctx, slog.LevelDebug, msg, attrs...)
}

// Info logs with LevelInfo using the current process-wide default logger.
func Info(msg string, attrs ...Attr) {
	InfoContext(context.Background(), msg, attrs...)
}

// InfoContext logs with LevelInfo using the current process-wide default logger.
// Context is passed through so context-bound attrs and tracing metadata can be attached.
func InfoContext(ctx context.Context, msg string, attrs ...Attr) {
	defaultLogger.Load().LogAttrs(ctx, slog.LevelInfo, msg, attrs...)
}

// Warn logs with LevelWarn using the current process-wide default logger.
func Warn(msg string, attrs ...Attr) {
	WarnContext(context.Background(), msg, attrs...)
}

// WarnContext logs with LevelWarn using the current process-wide default logger.
// Context is passed through so context-bound attrs and tracing metadata can be attached.
func WarnContext(ctx context.Context, msg string, attrs ...Attr) {
	defaultLogger.Load().LogAttrs(ctx, slog.LevelWarn, msg, attrs...)
}

// Error logs with LevelError using the current process-wide default logger.
func Error(msg string, attrs ...Attr) {
	ErrorContext(context.Background(), msg, attrs...)
}

// ErrorContext logs with LevelError using the current process-wide default logger.
// Context is passed through so context-bound attrs and tracing metadata can be attached.
func ErrorContext(ctx context.Context, msg string, attrs ...Attr) {
	defaultLogger.Load().LogAttrs(ctx, slog.LevelError, msg, attrs...)
}
