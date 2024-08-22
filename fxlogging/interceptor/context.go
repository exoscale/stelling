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

// traceIdFromContext will extract a traceid from the context, if any
// It will look for one in this order:
// 1. A trace-id set using contextWithTraceId
// 2. The OTEL trace-id from the context
// 3. A new random trace-id
// If a new trace-id was generated, the second return argument of this function
// will return 'false'. It is recommended to save this id on the context
// so that future calls produce the same trace-id
func traceIdFromContext(ctx context.Context) (string, bool) {
	id := ctx.Value(traceIdCtxKey)
	if id != nil {
		idstr, ok := id.(string)
		if ok && idstr != "" {
			return idstr, true
		}
	}
	spanCtx := oteltrace.SpanContextFromContext(ctx)
	if spanCtx.HasTraceID() {
		return spanCtx.TraceID().String(), true
	}
	return fmt.Sprintf("local-%s", ulid.Make()), false
}

// ContextWithLogger returns a copy of the given context with a Logger embedded into it
func ContextWithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerCtxKey, logger)
}

// LoggerFromContext extracts the zap Logger from the given context
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
	return logger
}
