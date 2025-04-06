package logger

import (
	"log/slog"
	"time"
)

type (
	// Attr type alias.
	Attr = slog.Attr
	// Level type alias.
	Level = slog.Level
	// Logger type alias.
	Logger = slog.Logger
	// Record type alias.
	Record = slog.Record
	// Handler type alias.
	Handler = slog.Handler
	// HandlerOptions type alias.
	HandlerOptions = slog.HandlerOptions
)

var ( // Predefined aliases for slog attribute functions.
	_ = String
	_ = Int64
	_ = Int
	_ = Uint64
	_ = Float64
	_ = Bool
	_ = Time
	_ = Duration
	_ = Group
	_ = Any
	_ = Err
	_ = NamedError
)

func newLogger(handler Handler) *Logger { return slog.New(handler) }

// String is an alias for slog.String.
func String(key, value string) Attr { return slog.String(key, value) }

// Int64 is an alias for slog.Int64.
func Int64(key string, value int64) Attr { return slog.Int64(key, value) }

// Int is an alias for slog.Int.
func Int(key string, value int) Attr { return slog.Int(key, value) }

// Uint64 is an alias for slog.Uint64.
func Uint64(key string, value uint64) Attr { return slog.Uint64(key, value) }

// Float64 is an alias for slog.Float64.
func Float64(key string, value float64) Attr { return slog.Float64(key, value) }

// Bool is an alias for slog.Bool.
func Bool(key string, value bool) Attr { return slog.Bool(key, value) }

// Time is an alias for slog.Time.
func Time(key string, value time.Time) Attr { return slog.Time(key, value) }

// Duration is an alias for slog.Duration.
func Duration(key string, value time.Duration) Attr { return slog.Duration(key, value) }

// Group is an alias for slog.Group.
func Group(key string, attrs ...any) Attr { return slog.Group(key, attrs...) }

// Any is an alias for slog.Any.
func Any(key string, value any) Attr { return slog.Any(key, value) }

// Err is a helper function to log errors using NamedError.
func Err(err error) slog.Attr { return NamedError("error", err) }

// NamedError creates a named error attribute for logging.
func NamedError(name string, err error) slog.Attr {
	if err == nil {
		return slog.Attr{}
	}

	return slog.Attr{
		Key:   name,
		Value: slog.AnyValue(err),
	}
}
