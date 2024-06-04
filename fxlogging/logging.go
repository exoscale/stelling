// Package fxlogging provides a convenient way to create loggers.
package fxlogging

import (
	"context"
	"time"

	"github.com/exoscale/stelling/fxgrpc"
	"github.com/exoscale/stelling/fxlogging/fxlogger"
	"github.com/exoscale/stelling/fxlogging/interceptor"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewModule provides a *zap.Logger to the system
// It also provides the following related items:
// * Grpc middleware
// * An adapter to log fx system events
func NewModule(conf LoggingConfig) fx.Option {
	return fx.Options(
		fx.WithLogger(fxlogger.NewFxLogger),
		fx.Module(
			"logging",
			fx.Provide(
				fx.Annotate(NewLogger, fx.ParamTags(``, ``, `group:"zap_opts"`)),
				fx.Annotate(
					NewGrpcLoggingServerInterceptors,
					fx.ParamTags(``, `group:"logging_server_interceptor_options"`),
					fx.ResultTags(`group:"unary_server_interceptor"`, `group:"stream_server_interceptor"`),
				),
				fx.Annotate(
					NewGrpcLoggingClientInterceptors,
					fx.ParamTags(``, `group:"logging_client_interceptor_options"`),
					fx.ResultTags(`group:"unary_client_interceptor"`, `group:"stream_client_interceptor"`),
				),
				fx.Annotate(
					NewGrpcInjectLoggerInterceptors,
					fx.ResultTags(`group:"unary_server_interceptor"`, `group:"stream_server_interceptor"`),
				),
				fx.Annotate(
					NewGrpcInjectPeerInterceptors,
					fx.ResultTags(`group:"unary_client_interceptor"`, `group:"stream_client_interceptor"`),
				),
			),
			fx.Supply(
				fx.Annotate(conf, fx.As(new(LoggingConfig))),
				fx.Private,
			),
		),
	)
}

type LoggingConfig interface {
	LoggingConfig() *Logging
}

// Logging contains the configuration options for the logging module
type Logging struct {
	// LogMode is the preset logging configuration
	Mode string `default:"development" validate:"oneof=production development preproduction"`
}

func (l *Logging) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if l == nil {
		return nil
	}

	enc.AddString("mode", l.Mode)

	return nil
}

func (l *Logging) LoggingConfig() *Logging {
	return l
}

func NewLogger(conf LoggingConfig, lc fx.Lifecycle, opts ...zap.Option) (*zap.Logger, error) {
	var config zap.Config
	switch conf.LoggingConfig().Mode {
	case "production":
		config = zap.NewProductionConfig()
		config.EncoderConfig.EncodeTime = ISO8601UTCTimeEncoder
	case "preproduction":
		config = zap.NewProductionConfig()
		config.Sampling = nil
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
		config.EncoderConfig.EncodeTime = ISO8601UTCTimeEncoder
	default:
		config = zap.NewDevelopmentConfig()
	}
	config.OutputPaths = []string{"stdout"}
	config.ErrorOutputPaths = []string{"stdout"}
	logger, err := config.Build(opts...)
	if err != nil {
		return nil, err
	}
	logger.Info("Using configuration", zap.Any("conf", conf))

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// By logging here we render the decorated config
			// If any components update the config we'll show those results here
			logger.Info("Final configuration", zap.Any("conf", conf))
			return nil
		},
		OnStop: func(ctx context.Context) error {
			_ = logger.Sync()
			return nil
		},
	})

	return logger, nil
}

// ISO8601UTCTimeEncoder is like zapcore.ISO8601TimeEncoder but sets
// the timezone to utc first
func ISO8601UTCTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	t = t.UTC()
	zapcore.ISO8601TimeEncoder(t, enc)
}

const GrpcInterceptorWeight uint = 50

func NewGrpcLoggingServerInterceptors(logger *zap.Logger, opts ...interceptor.Option) (*fxgrpc.UnaryServerInterceptor, *fxgrpc.StreamServerInterceptor) {
	unaryIx := &fxgrpc.UnaryServerInterceptor{Weight: GrpcInterceptorWeight, Interceptor: interceptor.NewLoggingUnaryServerInterceptor(logger, opts...)}
	streamIx := &fxgrpc.StreamServerInterceptor{Weight: GrpcInterceptorWeight, Interceptor: interceptor.NewLoggingStreamServerInterceptor(logger, opts...)}
	return unaryIx, streamIx
}

func NewGrpcLoggingClientInterceptors(logger *zap.Logger, opts ...interceptor.Option) (*fxgrpc.UnaryClientInterceptor, *fxgrpc.StreamClientInterceptor) {
	logger = logger.WithOptions(zap.WithCaller(false))

	unaryIx := &fxgrpc.UnaryClientInterceptor{Weight: GrpcInterceptorWeight, Interceptor: interceptor.NewLoggingUnaryClientInterceptor(logger, opts...)}
	streamIx := &fxgrpc.StreamClientInterceptor{Weight: GrpcInterceptorWeight, Interceptor: interceptor.NewLoggingStreamClientInterceptor(logger, opts...)}
	return unaryIx, streamIx
}

func NewGrpcInjectLoggerInterceptors(logger *zap.Logger) (*fxgrpc.UnaryServerInterceptor, *fxgrpc.StreamServerInterceptor) {
	weight := GrpcInterceptorWeight - 1
	unaryIx := &fxgrpc.UnaryServerInterceptor{Weight: weight, Interceptor: interceptor.NewInjectLoggerUnaryServerInterceptor(logger)}
	streamIx := &fxgrpc.StreamServerInterceptor{Weight: weight, Interceptor: interceptor.NewInjectLoggerStreamServerInterceptor(logger)}
	return unaryIx, streamIx
}

func NewGrpcInjectPeerInterceptors() (*fxgrpc.UnaryClientInterceptor, *fxgrpc.StreamClientInterceptor) {
	weight := GrpcInterceptorWeight - 1
	unaryIx := &fxgrpc.UnaryClientInterceptor{Weight: weight, Interceptor: interceptor.NewInjectPeerUnaryClientInterceptor()}
	streamIx := &fxgrpc.StreamClientInterceptor{Weight: weight, Interceptor: interceptor.NewInjectPeerStreamClientInterceptor()}
	return unaryIx, streamIx
}
