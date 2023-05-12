package interceptor

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// NewInjectLoggerUnaryServerInterceptor returns a UnaryServerInterceptor that stores a *zap.Logger
// enriched with a trace-id in the request context
// The handler can obtain the logger by calling `LoggerFromContext`
func NewInjectLoggerUnaryServerInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		traceid, ok := traceIdFromContext(ctx)
		if !ok {
			ctx = contextWithTraceId(ctx, traceid)
		}
		logger = logger.With(zap.String("otlp.trace_id", traceid))
		ctx = ContextWithLogger(ctx, logger)

		return handler(ctx, req)
	}
}

type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *wrappedServerStream) Context() context.Context {
	return s.ctx
}

// NewInjectLoggerStreamServerInterceptor returns a StreamServerInterceptor that stores a *zap.Logger
// enriched with a trace-id in the request context
// The handler can obtain the logger by calling `LoggerFromContext`
func NewInjectLoggerStreamServerInterceptor(logger *zap.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()

		traceid, ok := traceIdFromContext(ctx)
		if !ok {
			ctx = contextWithTraceId(ctx, traceid)
		}
		logger = logger.With(zap.String("otlp.trace_id", traceid))
		ctx = ContextWithLogger(ctx, logger)

		wrappedStream := &wrappedServerStream{ctx: ctx, ServerStream: ss}

		return handler(srv, wrappedStream)
	}
}
