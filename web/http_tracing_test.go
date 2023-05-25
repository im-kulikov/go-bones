package web

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracer "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

// newExporter returns a console exporter.
func newExporter(w io.Writer) (*stdouttrace.Exporter, error) {
	return stdouttrace.New(
		stdouttrace.WithWriter(w),
		// Use human-readable output.
		stdouttrace.WithPrettyPrint(),
		// Do not print timestamps for the demo.
		stdouttrace.WithoutTimestamps(),
	)
}

// newResource returns a resource describing this application.
func newResource() *resource.Resource {
	r, _ := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("fib"),
			semconv.ServiceVersionKey.String("v0.1.0"),
			attribute.String("environment", "demo"),
		),
	)
	return r
}

func TestHTTPTracing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	buf := new(bytes.Buffer)

	exp, err := newExporter(buf)
	require.NoError(t, err)

	tp := tracer.NewTracerProvider(
		tracer.WithBatcher(exp),
		tracer.WithResource(newResource()))

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	stx, span := otel.Tracer("from-test").Start(ctx, "came from test")
	defer span.End()

	srv := httptest.NewServer(HTTPTracingMiddlewareFunc(func(w http.ResponseWriter, r *http.Request) {
		current := trace.SpanFromContext(r.Context())

		assert.Equal(t, span.SpanContext().TraceID(), current.SpanContext().TraceID())
	}))

	cli := srv.Client()
	ApplyTracingToHTTPClient(cli)

	req, err := http.NewRequestWithContext(stx, http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	res, err := cli.Do(req)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	require.NoError(t, tp.Shutdown(ctx))
	require.Contains(t, buf.String(), span.SpanContext().TraceID().String())
}
