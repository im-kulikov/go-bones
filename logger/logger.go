package logger

import (
	"context"
	"log"
	"os"
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
)

// Config structure that provides configuration of logger module.
type Config struct {
	EncodingConsole bool   `env:"ENCODING_CONSOLE" default:"false" usage:"allows to set user-friendly formatting"`
	Level           string `env:"LEVEL" default:"info" usage:"allows to set custom logger level"`
	Trace           string `env:"TRACE" default:"fatal" usage:"allows to set custom trace level"`
	SampleRate      *int   `env:"SAMPLE_RATE" default:"1000" usage:"allows to set sample rate"`
}

type testingT interface {
	Helper()
	zaptest.TestingT
}

type logger struct {
	appName    string
	appVersion string

	colored bool

	config  zap.Config
	options []zap.Option

	*SugaredLogger
}

// Validate we should check that passed configuration is valid, so:
// - trace and level should be empty or valid logger level
// - sample rate should be empty or greater than zero.
func (c *Config) Validate(_ context.Context) error {
	err := validation.ValidateStruct(c,
		validation.Field(&c.SampleRate, validation.NilOrNotEmpty),
		validation.Field(&c.Level, validation.Required),
		validation.Field(&c.Level, validation.Required, validation.In(allLevels...)),
		validation.Field(&c.Trace, validation.Required),
		validation.Field(&c.Trace, validation.Required, validation.In(allLevels...)))
	if err != nil {
		return err
	}

	return nil
}

// With allows to provide zap.SugaredLogger as common interface.
func (l *logger) With(args ...interface{}) Logger {
	return &logger{
		config:        l.config,
		appName:       l.appName,
		appVersion:    l.appVersion,
		SugaredLogger: l.SugaredLogger.With(args...),
	}
}

// Named allows to set name for zap.SugaredLogger.
func (l *logger) Named(name string) Logger {
	return &logger{
		config:        l.config,
		appName:       l.appName,
		appVersion:    l.appVersion,
		SugaredLogger: l.SugaredLogger.Named(name),
	}
}

// Sugar returns zap.SugaredLogger.
func (l *logger) Sugar() *SugaredLogger { return l.SugaredLogger }

// Std returns standard library log.Logger.
func (l *logger) Std() *log.Logger { return zap.NewStdLog(l.Desugar()) }

// nolint: gochecknoglobals
var allLevels = []interface{}{
	zapcore.InfoLevel.String(),
	zapcore.DebugLevel.String(),
	zapcore.WarnLevel.String(),
	zapcore.ErrorLevel.String(),
	zapcore.DPanicLevel.String(),
	zapcore.PanicLevel.String(),
	zapcore.FatalLevel.String(),
}

// safeLevel converts string representation into log level.
func safeLevel(level string) zapcore.Level {
	switch strings.ToLower(level) {
	default:
		return zapcore.InfoLevel
	case "debug":
		return zapcore.DebugLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "dpanic":
		return zapcore.DPanicLevel
	case "panic":
		return zapcore.PanicLevel
	case "fatal":
		return zapcore.FatalLevel
	}
}

// nolint: gochecknoglobals
var defaultSampleRate = 1000

// Default returns default logger instance.
func Default() Logger {
	atom := zap.NewAtomicLevel()
	atom.SetLevel(zapcore.DebugLevel)

	encoderCfg := zap.NewProductionEncoderConfig()

	encoderCfg.TimeKey = "ts"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	// Default JSON encoder
	encoder := zapcore.NewJSONEncoder(encoderCfg)

	l := zap.New(zapcore.NewCore(
		encoder,
		zapcore.Lock(os.Stdout),
		atom),
		zap.AddCaller(),
	)

	return &logger{SugaredLogger: l.Sugar()}
}

// ForTests wrapped logger for tests.
func ForTests(t testingT) Logger {
	t.Helper()

	return &logger{SugaredLogger: zaptest.NewLogger(t).Sugar()}
}

// New prepares logger module.
func New(cfg Config, opts ...Option) (Logger, error) {
	var err error
	logLevel := safeLevel(cfg.Level)
	logTrace := safeLevel(cfg.Trace)

	var l logger
	l.config = zap.NewProductionConfig()

	l.config.Level = zap.NewAtomicLevelAt(logLevel)

	l.config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	if cfg.EncodingConsole {
		l.config.Encoding = "console"
	}

	for _, o := range opts {
		o(&l)
	}

	if cfg.EncodingConsole && l.colored {
		l.config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	if cfg.SampleRate == nil {
		cfg.SampleRate = &defaultSampleRate
	}

	l.config.Sampling.Initial = *cfg.SampleRate
	l.config.Sampling.Thereafter = *cfg.SampleRate

	var zapLogger *zap.Logger
	if zapLogger, err = l.config.Build(zap.AddStacktrace(logTrace)); err != nil {
		return nil, err
	}

	if l.appName != "" {
		zapLogger = zapLogger.With(zap.String("app", l.appName))
	}

	if l.appVersion != "" {
		zapLogger = zapLogger.With(zap.String("version", l.appVersion))
	}

	l.SugaredLogger = zapLogger.Sugar()

	return &l, nil
}
