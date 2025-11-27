package interceptor

import (
	"context"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// NewInjectLoggerUnaryServerInterceptor returns a UnaryServerInterceptor that stores a *zap.Logger
// enriched with a trace-id and optional metadata-derived fields in the request context
// The handler can obtain the logger by calling `LoggerFromContext`

func NewInjectLoggerUnaryServerInterceptor(logger *zap.Logger, opts ...Option) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		conf := newInterceptorConfig(opts)
		interceptorInfo := &otelgrpc.InterceptorInfo{UnaryServerInfo: info, Type: otelgrpc.UnaryServer}

		newLogger := loggerWithDefaultFields(ctx, logger, interceptorInfo)
		newLogger = loggerWithMetadata(ctx, newLogger, conf.metadataFields)
		newLogger = conf.extraFieldsFunc(newLogger, interceptorInfo, req)

		ctx = ContextWithLogger(ctx, newLogger)

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
func NewInjectLoggerStreamServerInterceptor(logger *zap.Logger, opts ...Option) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		conf := newInterceptorConfig(opts)
		interceptorInfo := &otelgrpc.InterceptorInfo{StreamServerInfo: info, Type: otelgrpc.StreamServer}

		ctx := ss.Context()
		mStream := &monitoredServerStream{ctx: ctx, ServerStream: ss}

		newLogger := loggerWithDefaultFields(ctx, logger, interceptorInfo)
		newLogger = loggerWithMetadata(ctx, newLogger, conf.metadataFields)
		newLogger = conf.extraFieldsFunc(newLogger, interceptorInfo, mStream.payload)

		ctx = ContextWithLogger(ctx, newLogger)

		wrappedStream := &wrappedServerStream{ctx: ctx, ServerStream: ss}

		return handler(srv, wrappedStream)
	}
}
