package fxlogging

import (
	"github.com/exoscale/stelling/fxlogging/fxlogger"
	"github.com/exoscale/stelling/fxlogging/interceptor"
	"go.uber.org/zap"
)

type moduleConfig struct {
	fxLoggerOptions              []fxlogger.Option
	zapOptions                   []zap.Option
	grpcServerInterceptorOptions []interceptor.Option
	grpcClientInterceptorOptions []interceptor.Option
	enableGrpcClientInterceptors bool
}

type Option func(*moduleConfig)

// WithZapOption will apply the given zap.Option to the logger provided by the module
// Can be given multiple times to supply different options
func WithZapOption(opt zap.Option) Option {
	return func(mc *moduleConfig) {
		mc.zapOptions = append(mc.zapOptions, opt)
	}
}

// WithFxLoggerOption will apply the fxlogger Option to the logger used by the fx machinery
// Can be given multiple times to supply different options
func WithFxLoggerOption(opt fxlogger.Option) Option {
	return func(mc *moduleConfig) {
		mc.fxLoggerOptions = append(mc.fxLoggerOptions, opt)
	}
}

// WithGrpcClientInterceptors determines if Grpc Client interceptors can be provided by the system
// This is true by default, but is undesirable in high volume use cases
// If you app does not use gRPC you can ignore this: fx provides constructors lazily
func WithGrpcClientInterceptors(enabled bool) Option {
	return func(mc *moduleConfig) {
		mc.enableGrpcClientInterceptors = enabled
	}
}

func WithGrpcServerInterceptorOptions(opt interceptor.Option) Option {
	return func(mc *moduleConfig) {
		mc.grpcServerInterceptorOptions = append(mc.grpcServerInterceptorOptions, opt)
	}
}

func WithGrpcClientInterceptorOptions(opt interceptor.Option) Option {
	return func(mc *moduleConfig) {
		mc.grpcClientInterceptorOptions = append(mc.grpcClientInterceptorOptions, opt)
	}
}
