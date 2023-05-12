package interceptor

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/codes"
)

type interceptorConfig struct {
	levelFunc       func(codes.Code) zapcore.Level
	logFilter       otelgrpc.Filter
	payloadFilter   otelgrpc.Filter
	extraFieldsFunc func(logger *zap.Logger, info *otelgrpc.InterceptorInfo, payload any) *zap.Logger
}

type Option func(*interceptorConfig)

// WithLevelFunc provides a custom implementation that maps a gRPC status code to a logging level
func WithLevelFunc(f func(codes.Code) zapcore.Level) Option {
	return func(c *interceptorConfig) {
		c.levelFunc = f
	}
}

// WithExtraFieldsFunc sets a callback that can be used to configure the logger
// It's main purpose is to set additional fields based on the rpc info and payload, but
// any modification can be made
// The payload here is the payload as would be logged when WithPayloadFilter returns true
func WithExtraFieldsFunc(f func(*zap.Logger, *otelgrpc.InterceptorInfo, any) *zap.Logger) Option {
	return func(c *interceptorConfig) {
		c.extraFieldsFunc = f
	}
}

// WithLogFilter registers a predicate to determine whether the request should be logged
// The predicate function must return `true` to log the request
func WithLogFilter(f otelgrpc.Filter) Option {
	return func(c *interceptorConfig) {
		c.logFilter = f
	}
}

// WithLogFilter registers a predicate to determine whether the request payload should be logged
// The predicate function must return `true` to log the request payload
func WithPayloadFilter(f otelgrpc.Filter) Option {
	return func(c *interceptorConfig) {
		c.payloadFilter = f
	}
}

func newInterceptorConfig(opts []Option) *interceptorConfig {
	conf := &interceptorConfig{
		levelFunc:       DefaultServerCodeToLevel,
		logFilter:       defaultFilter,
		payloadFilter:   defaultPayloadFilter,
		extraFieldsFunc: defaultExtraFieldsFunc,
	}

	for _, opt := range opts {
		opt(conf)
	}

	return conf
}

func defaultExtraFieldsFunc(logger *zap.Logger, _ *otelgrpc.InterceptorInfo, _ any) *zap.Logger {
	return logger
}

func defaultFilter(_ *otelgrpc.InterceptorInfo) bool {
	return true
}

func defaultPayloadFilter(_ *otelgrpc.InterceptorInfo) bool {
	return false
}

func DefaultServerCodeToLevel(code codes.Code) zapcore.Level {
	switch code {
	case codes.OK:
		return zap.InfoLevel
	case codes.Canceled:
		return zap.InfoLevel
	case codes.Unknown:
		return zap.ErrorLevel
	case codes.InvalidArgument:
		return zap.InfoLevel
	case codes.DeadlineExceeded:
		return zap.WarnLevel
	case codes.NotFound:
		return zap.InfoLevel
	case codes.AlreadyExists:
		return zap.DebugLevel
	case codes.PermissionDenied:
		return zap.WarnLevel
	case codes.Unauthenticated:
		return zap.InfoLevel // unauthenticated requests can happen
	case codes.ResourceExhausted:
		return zap.WarnLevel
	case codes.FailedPrecondition:
		return zap.WarnLevel
	case codes.Aborted:
		return zap.WarnLevel
	case codes.OutOfRange:
		return zap.WarnLevel
	case codes.Unimplemented:
		return zap.ErrorLevel
	case codes.Internal:
		return zap.ErrorLevel
	case codes.Unavailable:
		return zap.ErrorLevel
	case codes.DataLoss:
		return zap.ErrorLevel
	default:
		return zap.ErrorLevel
	}
}

func DefaultClientCodeToLevel(code codes.Code) zapcore.Level {
	switch code {
	case codes.OK:
		return zap.DebugLevel
	case codes.Canceled:
		return zap.DebugLevel
	case codes.Unknown:
		return zap.ErrorLevel
	case codes.InvalidArgument:
		return zap.InfoLevel
	case codes.DeadlineExceeded:
		return zap.WarnLevel
	case codes.NotFound:
		return zap.InfoLevel
	case codes.AlreadyExists:
		return zap.DebugLevel
	case codes.PermissionDenied:
		return zap.WarnLevel
	case codes.Unauthenticated:
		return zap.InfoLevel // unauthenticated requests can happen
	case codes.ResourceExhausted:
		return zap.WarnLevel
	case codes.FailedPrecondition:
		return zap.WarnLevel
	case codes.Aborted:
		return zap.WarnLevel
	case codes.OutOfRange:
		return zap.WarnLevel
	case codes.Unimplemented:
		return zap.ErrorLevel
	case codes.Internal:
		return zap.ErrorLevel
	case codes.Unavailable:
		return zap.ErrorLevel
	case codes.DataLoss:
		return zap.ErrorLevel
	default:
		return zap.ErrorLevel
	}
}
