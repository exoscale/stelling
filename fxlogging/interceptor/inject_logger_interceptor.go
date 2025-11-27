package interceptor

import (
	"context"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// NewInjectLoggerUnaryServerInterceptor returns a UnaryServerInterceptor that stores a *zap.Logger
// enriched with a trace-id and optional metadata-derived fields in the request context
// The handler can obtain the logger by calling `LoggerFromContext`

func NewInjectLoggerUnaryServerInterceptor(logger *zap.Logger, opts ...Option) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		conf := newInterceptorConfig(opts)
		interceptorInfo := &otelgrpc.InterceptorInfo{UnaryServerInfo: info, Type: otelgrpc.UnaryServer}
		service, method := MethodFromInterceptorInfo(interceptorInfo)

		traceid, ok := traceIdFromContext(ctx)
		if !ok {
			ctx = contextWithTraceId(ctx, traceid)
		}

		newLogger := logger.With(
			zap.String("otlp.trace_id", traceid),
			zap.String("rpc.system", "grpc"),
			zap.String("service.name", serviceName()),
			zap.String("rpc.method", method),
			zap.String("rpc.service", service))
		newLogger = enrichLoggerWithMetadata(ctx, newLogger, conf.metadataFields)

		ctx = ContextWithLogger(ctx, conf.extraFieldsFunc(newLogger, interceptorInfo, req))

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
		service, method := MethodFromInterceptorInfo(interceptorInfo)

		ctx := ss.Context()
		mStream := &monitoredServerStream{ctx: ctx, ServerStream: ss}

		traceid, ok := traceIdFromContext(ctx)
		if !ok {
			ctx = contextWithTraceId(ctx, traceid)
		}
		newLogger := logger.With(
			zap.String("otlp.trace_id", traceid),
			zap.String("rpc.system", "grpc"),
			zap.String("service.name", serviceName()),
			zap.String("rpc.method", method),
			zap.String("rpc.service", service))
		newLogger = enrichLoggerWithMetadata(ctx, newLogger, conf.metadataFields)
		ctx = ContextWithLogger(ctx, conf.extraFieldsFunc(newLogger, interceptorInfo, mStream.payload))

		wrappedStream := &wrappedServerStream{ctx: ctx, ServerStream: ss}

		return handler(srv, wrappedStream)
	}
}

// enrichLoggerWithMetadata extracts values from gRPC incoming metadata and adds them to the
// provided logger as string fields. mdFields maps metadata key -> logger field key.
// If mdFields is nil or empty, the logger is returned unchanged.
func enrichLoggerWithMetadata(ctx context.Context, logger *zap.Logger, mdFields map[string]string) *zap.Logger {
	if len(mdFields) == 0 {
		return logger
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return logger
	}
	for mdKey, fieldKey := range mdFields {
		if vals := md.Get(mdKey); len(vals) > 0 {
			logger = logger.With(zap.String(fieldKey, vals[0]))
		}
	}
	return logger
}
