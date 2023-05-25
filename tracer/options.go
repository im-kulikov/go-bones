package tracer

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

// Option allows to set custom options.
type Option func(*customOptions)

type customOptions struct {
	jaegerResourceOptions []attribute.KeyValue
	customErrorHandleFunc otel.ErrorHandlerFunc
}

// WithJaegerServiceName allows to set jaeger service name.
func WithJaegerServiceName(v string) Option {
	return func(j *customOptions) {
		j.jaegerResourceOptions = append(j.jaegerResourceOptions, semconv.ServiceName(v))
	}
}

// WithJaegerServiceVersion allows to set jaeger service version.
func WithJaegerServiceVersion(v string) Option {
	return func(j *customOptions) {
		j.jaegerResourceOptions = append(j.jaegerResourceOptions, semconv.ServiceVersion(v))
	}
}

// WithJaegerServiceEnv allows to set jaeger service environment.
func WithJaegerServiceEnv(v string) Option {
	return func(j *customOptions) {
		j.jaegerResourceOptions = append(j.jaegerResourceOptions, attribute.String("environment", v))
	}
}

// WithCustomErrorHandler allows to set custom otel.ErrorHandlerFunc.
func WithCustomErrorHandler(v otel.ErrorHandlerFunc) Option {
	return func(j *customOptions) { j.customErrorHandleFunc = v }
}
