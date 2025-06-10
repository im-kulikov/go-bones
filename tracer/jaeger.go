package tracer

import (
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

type jaegerSettings struct {
	ClientType string `env:"OTLP_CLIENT_TYPE"`
}

var (
	_ = newJaegerExporter                   // TODO fix after refactoring
	_ = new(jaegerSettings).exporterOptions // TODO fix after refactoring
)

func (j jaegerSettings) exporterOptions() {}

func newJaegerExporter(_ settings) {
	var test jaegerSettings
	resource.NewSchemaless(semconv.ServiceNameKey.String("otlp"))

	_ = test
}
