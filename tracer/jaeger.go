package tracer

import (
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

type jaegerSettings struct {
	ClientType string `env:"OTLP_CLIENT_TYPE"`
}

func (j jaegerSettings) exporterOptions() {}

func newJaegerExporter(cfg settings) {

	var test jaegerSettings // ...................................................................................................
	resource.NewSchemaless(semconv.ServiceNameKey.String("otlp"))

	_ = test

}
