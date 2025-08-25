// Package logger provides a convenient wrapper around slog with type aliases and helper functions
// for structured logging.
package logger

import (
	"log/slog"
	"time"
)

type (
	// Attr represents a key-value pair in a log record.
	Attr = slog.Attr

	// Level defines the severity of a log message.
	Level = slog.Level

	// Logger provides the main interface for structured logging.
	Logger = slog.Logger

	// Record represents a complete log message with its metadata and attributes.
	Record = slog.Record

	// Handler processes log records for output.
	Handler = slog.Handler

	// HandlerOptions configures the behavior of a Handler.
	HandlerOptions = slog.HandlerOptions
)

func newLogger(handler Handler) *Logger { return slog.New(handler) }

// String creates a string attribute.
// Parameters:
//   - key: The attribute name
//   - value: The string value
func String(key, value string) Attr { return slog.String(key, value) }

// Int64 creates an int64 attribute.
// Parameters:
//   - key: The attribute name
//   - value: The int64 value
func Int64(key string, value int64) Attr { return slog.Int64(key, value) }

// Int creates an integer attribute.
// Parameters:
//   - key: The attribute name
//   - value: The integer value
func Int(key string, value int) Attr { return slog.Int(key, value) }

// Uint64 creates an unsigned 64-bit integer attribute.
// Parameters:
//   - key: The attribute name
//   - value: The uint64 value
func Uint64(key string, value uint64) Attr { return slog.Uint64(key, value) }

// Float64 creates a float64 attribute.
// Parameters:
//   - key: The attribute name
//   - value: The float64 value
func Float64(key string, value float64) Attr { return slog.Float64(key, value) }

// Bool creates a boolean attribute.
// Parameters:
//   - key: The attribute name
//   - value: The boolean value
func Bool(key string, value bool) Attr { return slog.Bool(key, value) }

// Time creates a timestamp attribute.
// Parameters:
//   - key: The attribute name
//   - value: The time value
func Time(key string, value time.Time) Attr { return slog.Time(key, value) }

// Duration creates a time duration attribute.
// Parameters:
//   - key: The attribute name
//   - value: The duration value
func Duration(key string, value time.Duration) Attr { return slog.Duration(key, value) }

// Group creates a group of attributes with the given key.
// Parameters:
//   - key: The group name
//   - attrs: The attributes to group together
func Group(key string, attrs ...any) Attr { return slog.Group(key, attrs...) }

// Any creates an attribute that can hold any value type.
// Parameters:
//   - key: The attribute name
//   - value: The value of any type
func Any(key string, value any) Attr { return slog.Any(key, value) }

// Err creates an error attribute with the default key "error".
// Returns an empty attribute if the error is nil.
func Err(err error) slog.Attr { return NamedError("error", err) }

// NamedError creates an error attribute with a custom key name.
// Returns an empty attribute if the error is nil.
// Parameters:
//   - name: The custom key name for the error
//   - err: The error value
func NamedError(name string, err error) slog.Attr {
	if err == nil {
		return slog.Attr{}
	}

	return slog.Attr{
		Key:   name,
		Value: slog.AnyValue(err),
	}
}
