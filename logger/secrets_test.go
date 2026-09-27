package logger

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/im-kulikov/go-bones/config"
)

// logSecrets writes one record that carries a secret in every supported place:
// a context attribute, a top-level attribute, a field inside a group, and a
// group whose own name is a secret. It returns the secret values that must not
// appear in any output.
func logSecrets(ctx context.Context, log *Logger) []string {
	log.InfoContext(
		AddContextAttrs(ctx, String("my-password", "ctx secret")),
		"hello world",
		String("my-password", "direct secret"),
		slog.Group("user",
			slog.String("my-password", "nested secret"),
			slog.String("name", "bob")),
		slog.Group("credentials", slog.String("login", "group secret")),
	)

	return []string{"ctx secret", "direct secret", "nested secret", "group secret"}
}

func Test_secretTransformer(t *testing.T) {
	buf := new(bytes.Buffer)
	log := ForTests(TestLoggerWriter(buf), TestLoggerSecrets("my-password", "credentials"))
	secrets := logSecrets(context.Background(), log)

	out := buf.String()
	for _, secret := range secrets {
		require.NotContains(t, out, secret)
	}

	require.Contains(t, out, "user.name=bob")
	require.Contains(t, out, "credentials=REDACTED")
}

func Test_secretTransformer_RunsBeforeExporters(t *testing.T) {
	t.Run("otel log bridge", func(t *testing.T) {
		processor := new(captureProcessor)
		installOTelLogProvider(t, processor)
		SetOpenTelemetryBridge(true)

		secrets := logSecrets(
			context.Background(),
			ForTests(TestLoggerSecrets("my-password", "credentials")),
		)

		require.Len(t, processor.records, 1)
		exported := fmt.Sprint(processor.records[0].Attributes)
		for _, secret := range secrets {
			require.NotContains(t, exported, secret)
		}
	})

	t.Run("span events", func(t *testing.T) {
		SetOpenTelemetryBridge(false)

		recorder := tracetest.NewSpanRecorder()
		provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
		t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

		ctx, span := provider.Tracer("test").Start(context.Background(), "main")
		secrets := logSecrets(ctx, ForTests(TestLoggerSecrets("my-password", "credentials")))
		span.End()

		spans := recorder.Ended()
		require.Len(t, spans, 1)
		require.NotEmpty(t, spans[0].Events())

		exported := fmt.Sprint(spans[0].Events())
		for _, secret := range secrets {
			require.NotContains(t, exported, secret)
		}
	})
}

func Test_secretTransformer_MasksAttrsFromCustomTransformers(t *testing.T) {
	buf := new(bytes.Buffer)
	addSecret := slogTransformerFunc(func(_ context.Context, record Record) Record {
		record.AddAttrs(String("my-password", "custom secret"))

		return record
	})

	log := New(
		config.Logger{Secrets: []string{"my-password"}},
		slog.NewTextHandler(buf, nil),
		addSecret,
	)
	log.Info("hello world")

	require.NotContains(t, buf.String(), "custom secret")
	require.Contains(t, buf.String(), "my-password=REDACTED")
}
