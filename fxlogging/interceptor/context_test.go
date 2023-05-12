package interceptor

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestTraceIdFromContext(t *testing.T) {
	t.Run("Should extract the trace-id set by contextWithTraceId", func(t *testing.T) {
		expected := "my-custom-trace-id"
		ctx := contextWithTraceId(context.Background(), expected)

		traceId, ok := traceIdFromContext(ctx)
		require.True(t, ok)
		require.Equal(t, expected, traceId)
	})

	t.Run("Should extract the trace-id set by otel tracing", func(t *testing.T) {
		// The NoopTracerProvider doesn't supply TraceIDs, so we can't use it
		// in this test
		exporter, err := stdouttrace.New()
		require.NoError(t, err)
		tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
		t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

		ctx, span := tp.Tracer("my-test").Start(context.Background(), "test")
		defer span.End()

		traceId, ok := traceIdFromContext(ctx)
		require.True(t, ok)
		require.NotEmpty(t, traceId)
		require.False(t, strings.HasPrefix(traceId, "local-"))
	})

	t.Run("Should generate a new trace-id", func(t *testing.T) {
		ctx := context.Background()
		traceId, ok := traceIdFromContext(ctx)
		require.False(t, ok)
		require.True(t, strings.HasPrefix(traceId, "local-"))
	})

	t.Run("Should prefer custom trace-id over otel trace-id", func(t *testing.T) {
		// The NoopTracerProvider doesn't supply TraceIDs, so we can't use it
		// in this test
		exporter, err := stdouttrace.New()
		require.NoError(t, err)
		tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
		t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

		ctx, span := tp.Tracer("my-test").Start(context.Background(), "test")
		defer span.End()

		expected := "my-custom-trace-id"
		ctx = contextWithTraceId(ctx, expected)

		traceId, ok := traceIdFromContext(ctx)
		require.True(t, ok)
		require.Equal(t, expected, traceId)
	})
}

func TestLoggerFromContext(t *testing.T) {
	t.Run("Should return a noop logger if there's no logger present", func(t *testing.T) {
		logger := LoggerFromContext(context.Background())
		require.NotNil(t, logger)
		// Works because zap creates a singleton instance of the nop logger
		require.Equal(t, zap.NewNop(), logger)
	})

	t.Run("Should return the logger set by ContextWithLogger", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		ctx := ContextWithLogger(context.Background(), logger)

		require.Equal(t, logger, LoggerFromContext(ctx))
	})
}
