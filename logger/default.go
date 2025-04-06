package logger

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/im-kulikov/go-bones/config"
)

// nolint:gochecknoglobals
var defaultLogger atomic.Pointer[Logger]

// Init allows to prepare default Logger.
func Init(cfg config.Logger, handler Handler, transformers ...slogTransformer) *Logger {
	logger := New(cfg, handler, transformers...)
	defaultLogger.Store(logger)

	return logger
}

// Debug emits a log record with the current time and Debug level and message.
// The Record's Attrs consist of the Logger's attributes followed by
// the Attrs specified by args.
func Debug(ctx context.Context, msg string, attrs ...Attr) {
	defaultLogger.Load().LogAttrs(ctx, slog.LevelDebug, msg, attrs...)
}

// Info emits a log record with the current time and Info level and message.
// The Record's Attrs consist of the Logger's attributes followed by
// the Attrs specified by args.
func Info(ctx context.Context, msg string, attrs ...Attr) {
	defaultLogger.Load().LogAttrs(ctx, slog.LevelInfo, msg, attrs...)
}

// Warn emits a log record with the current time and Warn level and message.
// The Record's Attrs consist of the Logger's attributes followed by
// the Attrs specified by args.
func Warn(ctx context.Context, msg string, attrs ...Attr) {
	defaultLogger.Load().LogAttrs(ctx, slog.LevelWarn, msg, attrs...)
}

// Error emits a log record with the current time and Error level and message.
// The Record's Attrs consist of the Logger's attributes followed by
// the Attrs specified by args.
func Error(ctx context.Context, msg string, attrs ...Attr) {
	defaultLogger.Load().LogAttrs(ctx, slog.LevelError, msg, attrs...)
}
