package interceptor

import (
	"context"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
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
	ctx  context.Context
	info *otelgrpc.InterceptorInfo
	conf *interceptorConfig
}

func (s *wrappedServerStream) Context() context.Context {
	return s.ctx
}

func (s *wrappedServerStream) RecvMsg(m any) error {
	err := s.ServerStream.RecvMsg(m)

	if err == nil {
		msg, ok := m.(proto.Message)
		if ok {
			logger := LoggerFromContext(s.ctx)
			logger = s.conf.extraFieldsFunc(logger, s.info, msg)
			s.ctx = ContextWithLogger(s.ctx, logger)
		}
	}

	return err
}

// NewInjectLoggerStreamServerInterceptor returns a StreamServerInterceptor that stores a *zap.Logger
// enriched with a trace-id in the request context
// The handler can obtain the logger by calling `LoggerFromContext`
func NewInjectLoggerStreamServerInterceptor(logger *zap.Logger, opts ...Option) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		conf := newInterceptorConfig(opts)
		interceptorInfo := &otelgrpc.InterceptorInfo{StreamServerInfo: info, Type: otelgrpc.StreamServer}

		ctx := ss.Context()

		newLogger := loggerWithDefaultFields(ctx, logger, interceptorInfo)
		newLogger = loggerWithMetadata(ctx, newLogger, conf.metadataFields)

		ctx = ContextWithLogger(ctx, newLogger)

		wrappedStream := &wrappedServerStream{ctx: ctx, ServerStream: ss, info: interceptorInfo, conf: conf}

		return handler(srv, wrappedStream)
	}
}
