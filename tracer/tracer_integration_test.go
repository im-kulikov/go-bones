package tracer

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/im-kulikov/go-bones/internal/testutil"
)

func TestServiceBootstrapUsesBridgeAndExportsTelemetry(t *testing.T) {
	testutil.RequireNetworkIntegration(t)

	app := newTestApp(t)

	app.processPayment()
	app.stop()

	exportedSpan := findExportedSpan(t, app.collector.firstTraceRequest(), "process-payment")
	exportedLog := findExportedLog(t, app.collector.firstLogRequest(), "payment completed")
	logAttrs := protoAttrs(exportedLog.Attributes)
	resourceAttrs := app.collector.resourceLogAttrs(t)

	t.Run("exports trace", func(t *testing.T) {
		require.Equal(t, "process-payment", exportedSpan.Name)
	})

	t.Run("exports logs via bridge", func(t *testing.T) {
		require.Equal(t, "A-42", logAttrs["order_id"])
		require.Equal(t, "true", logAttrs["bridge_expected"])
	})

	t.Run("correlates log with active span", func(t *testing.T) {
		require.Equal(
			t,
			hex.EncodeToString(exportedSpan.TraceId),
			hex.EncodeToString(exportedLog.TraceId),
		)
		require.Equal(
			t,
			hex.EncodeToString(exportedSpan.SpanId),
			hex.EncodeToString(exportedLog.SpanId),
		)
		require.Equal(t, hex.EncodeToString(exportedLog.TraceId), logAttrs["trace_id"])
		require.Equal(t, hex.EncodeToString(exportedLog.SpanId), logAttrs["span_id"])
	})

	t.Run("uses config app metadata as resource fallback", func(t *testing.T) {
		require.Equal(t, "payments-api", resourceAttrs[string(semconv.ServiceNameKey)])
		require.Equal(t, "1.2.3", resourceAttrs[string(semconv.ServiceVersionKey)])
	})

	t.Run("does not duplicate span events", func(t *testing.T) {
		require.Empty(t, exportedSpan.Events, "bridge should replace span.AddEvent export path")
	})

	testutil.WriteArtifact(t, "tracer-export-summary.txt", []byte(fmt.Sprintf(
		"trace_name=%s\nlog_body=%s\ntrace_id=%s\nspan_id=%s\n",
		exportedSpan.Name,
		exportedLog.Body.GetStringValue(),
		hex.EncodeToString(exportedLog.TraceId),
		hex.EncodeToString(exportedLog.SpanId),
	)))
}
