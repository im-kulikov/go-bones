package logger

import (
	"log"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// WriteSyncer is an io.Writer that can also flush any buffered data. Note
// that *os.File (and thus, os.Stderr and os.Stdout) implement WriteSyncer.
// Type alias.
type WriteSyncer = zapcore.WriteSyncer

// Sink defines the interface to write to and close logger destinations.
// Type alias.
type Sink = zap.Sink

// A SugaredLogger wraps the base Logger functionality in a slower, but less
// verbose, API. Any Logger can be converted to a SugaredLogger with its Sugar
// method.
// Type alias.
type SugaredLogger = zap.SugaredLogger

// Logger common interface.
type Logger interface {
	Debug(...interface{})
	Debugf(msg string, args ...interface{})
	Debugw(msg string, keysAndValues ...interface{})

	Info(...interface{})
	Infof(msg string, args ...interface{})
	Infow(msg string, keysAndValues ...interface{})

	Warn(...interface{})
	Warnf(msg string, args ...interface{})
	Warnw(msg string, keysAndValues ...interface{})

	Error(...interface{})
	Errorf(msg string, args ...interface{})
	Errorw(msg string, keysAndValues ...interface{})

	Fatal(...interface{})
	Fatalf(msg string, args ...interface{})
	Fatalw(msg string, keysAndValues ...interface{})

	Panic(...interface{})
	Panicf(msg string, args ...interface{})
	Panicw(msg string, keysAndValues ...interface{})

	With(...interface{}) Logger

	Named(string) Logger

	Sugar() *SugaredLogger

	Std() *log.Logger

	Sync() error
}
