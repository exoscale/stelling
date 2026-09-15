package interceptor

import (
	"context"
	"fmt"

	ulid "github.com/oklog/ulid/v2"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type loggerContextKey struct{}
type traceIdContextKey struct{}

var loggerCtxKey = &loggerContextKey{}
var traceIdCtxKey = &traceIdContextKey{}
var nopLogger = zap.NewNop()

// contextWithTraceId returns a new trace-id that embeds the given trace-id
// It can be extracted again using the traceIdFromContext function.
func contextWithTraceId(ctx context.Context, traceid string) context.Context {
	return context.WithValue(ctx, traceIdCtxKey, traceid)
}

// traceInfoFromContext will extract a traceid and spanid from the context, if any
// It will look for one in this order:
// 1. The OTEL trace-id from the context
// 2. A trace-id set using contextWithTraceId
// 3. A new random trace-id
// If a new trace-id was generated, the second return argument of this function
// will return 'false'. It is recommended to save this id on the context
// so that future calls produce the same trace-id
func traceInfoFromContext(ctx context.Context) (string, string, bool) {
	spanCtx := oteltrace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		return spanCtx.TraceID().String(), spanCtx.SpanID().String(), true
	}
	id := ctx.Value(traceIdCtxKey)
	if id != nil {
		idstr, ok := id.(string)
		if ok && idstr != "" {
			return idstr, "", true
		}
	}
	return fmt.Sprintf("local-%s", ulid.Make()), "", false
}

// ContextWithLogger returns a copy of the given context with a Logger embedded into it
func ContextWithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerCtxKey, logger)
}

// LoggerFromContext extracts the zap Logger from the given context
// Sets the current trace_id and span_id from context, if any
// If no Logger is present, a NopLogger is returned
// Will never return nil
func LoggerFromContext(ctx context.Context) *zap.Logger {
	l := ctx.Value(loggerCtxKey)
	if l == nil {
		return nopLogger
	}
	logger, ok := l.(*zap.Logger)
	if !ok {
		return nopLogger
	}
	// We only cover this case, to get the actual spanId
	// If there's no spanId, then the traceId is typically fixed when the logger
	// is injected into context, and we don't need to try and enrich it again
	spanCtx := oteltrace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		return logger.With(
			zap.String("trace_id", spanCtx.TraceID().String()),
			zap.String("span_id", spanCtx.SpanID().String()),
		)
	}
	return logger
}
