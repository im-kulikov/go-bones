package logger

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/im-kulikov/go-bones/config"
)

// defaultLogger is a globally accessible logger instance that can be shared across the application.
// nolint:gochecknoglobals
var defaultLogger atomic.Pointer[Logger]

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
	for _, option := range opts {
		option(&o)
	}

	o.setDefaults()

	logger := New(cfg, o.handler, o.transformers...)
	defaultLogger.Store(logger)

	return logger
}

// Debug logs a message with LevelDebug severity using the default logger.
// This is typically used for development and debugging purposes.
//
// Parameters:
//   - ctx: The context containing additional metadata, such as deadlines or context-specific attributes.
//   - msg: The message to log.
//   - attrs: Additional attributes to include in the log record.
func Debug(ctx context.Context, msg string, attrs ...Attr) {
	defaultLogger.Load().LogAttrs(ctx, slog.LevelDebug, msg, attrs...)
}

// Info logs a message with LevelInfo severity using the default logger.
// This is typically used for general informational messages.
//
// Parameters:
//   - ctx: The context containing additional metadata, such as deadlines or context-specific attributes.
//   - msg: The message to log.
//   - attrs: Additional attributes to include in the log record.
func Info(ctx context.Context, msg string, attrs ...Attr) {
	defaultLogger.Load().LogAttrs(ctx, slog.LevelInfo, msg, attrs...)
}

// Warn logs a message with LevelWarn severity using the default logger.
// This is typically used to indicate something unexpected but not necessarily an error.
//
// Parameters:
//   - ctx: The context containing additional metadata, such as deadlines or context-specific attributes.
//   - msg: The message to log.
//   - attrs: Additional attributes to include in the log record.
func Warn(ctx context.Context, msg string, attrs ...Attr) {
	defaultLogger.Load().LogAttrs(ctx, slog.LevelWarn, msg, attrs...)
}

// Error logs a message with LevelError severity using the default logger.
// This is typically used to record error events.
//
// Parameters:
//   - ctx: The context containing additional metadata, such as deadlines or context-specific attributes.
//   - msg: The message to log.
//   - attrs: Additional attributes to include in the log record.
func Error(ctx context.Context, msg string, attrs ...Attr) {
	defaultLogger.Load().LogAttrs(ctx, slog.LevelError, msg, attrs...)
}
