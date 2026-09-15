package interceptor

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"
)

func TestTraceInfoFromContext(t *testing.T) {
	t.Run("Should extract the trace-id set by contextWithTraceId", func(t *testing.T) {
		expected := "my-custom-trace-id"
		ctx := contextWithTraceId(context.Background(), expected)

		traceId, spanId, ok := traceInfoFromContext(ctx)
		require.True(t, ok)
		require.Equal(t, expected, traceId)
		require.Empty(t, spanId)
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

		traceId, spanId, ok := traceInfoFromContext(ctx)
		require.True(t, ok)
		require.NotEmpty(t, traceId)
		require.NotEmpty(t, spanId)
		require.False(t, strings.HasPrefix(traceId, "local-"))
	})

	t.Run("Should generate a new trace-id", func(t *testing.T) {
		ctx := context.Background()
		traceId, spanId, ok := traceInfoFromContext(ctx)
		require.False(t, ok)
		require.True(t, strings.HasPrefix(traceId, "local-"))
		require.Empty(t, spanId)
	})

	t.Run("Should prefer otel trace-id over custom trace-id", func(t *testing.T) {
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

		traceId, spanId, ok := traceInfoFromContext(ctx)
		require.True(t, ok)
		require.NotEmpty(t, traceId)
		require.NotEmpty(t, spanId)
		require.False(t, strings.HasPrefix(traceId, "local-"))
		require.NotEqual(t, expected, traceId)
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

	t.Run("Should enrich with otel span context if present", func(t *testing.T) {
		// Injecting the logger first to ensure it correctly overwrites any traceId and spanId
		core, observer := observer.New(zapcore.DebugLevel)
		logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.WrapCore(func(_ zapcore.Core) zapcore.Core { return core }))).With(
			zap.String("trace_id", "foo"),
			zap.String("span_id", "bar"),
		)
		ctx := ContextWithLogger(context.Background(), logger)

		// The NoopTracerProvider doesn't supply TraceIDs, so we can't use it
		// in this test
		exporter, err := stdouttrace.New()
		require.NoError(t, err)
		tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
		t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

		ctx, span := tp.Tracer("my-test").Start(ctx, "test")
		defer span.End()

		ctxLogger := LoggerFromContext(ctx)
		ctxLogger.Debug("test")
		logs := observer.TakeAll()
		require.Len(t, logs, 1)
		log := logs[0]
		require.NotEmpty(t, log.ContextMap()["trace_id"])
		require.NotEqual(t, "foo", log.ContextMap()["trace_id"])
		require.NotEmpty(t, log.ContextMap()["span_id"])
		require.NotEqual(t, "bar", log.ContextMap()["span_id"])
	})
}
