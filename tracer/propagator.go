package tracer

import (
	"go.opentelemetry.io/contrib/propagators/jaeger"
)

// JaegerPropagator propagator serializes SpanContext to/from Jaeger Headers
// Jaeger format: uber-trace-id: {trace-id}:{span-id}:{parent-span-id}:{flags}.
// Type alias.
type JaegerPropagator = jaeger.Jaeger
