// Package fxlogging provides a convenient way to create loggers.
package fxlogging

import (
	"context"
	"time"

	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// NewModule provides a *zap.Logger to the system
// It also provides the following related items:
// * Grpc middleware
// * An adapter to log fx system events
func NewModule(conf LoggingConfig) fx.Option {
	return fx.Options(
		fx.WithLogger(NewFxLogger),
		fx.Module(
			"logging",
			fx.Provide(
				fx.Annotate(NewLogger, fx.ParamTags(``, ``, `group:"zap_opts"`)),
				fx.Annotate(
					NewGrpcServerInterceptors,
					fx.ParamTags(``, `group:"grpc_zap_server_options"`),
					fx.ResultTags(`group:"unary_server_interceptor"`, `group:"stream_server_interceptor"`),
				),
				fx.Annotate(
					NewGrpcClientInterceptors,
					fx.ParamTags(``, `group:"grpc_zap_client_options"`),
					fx.ResultTags(`group:"unary_client_interceptor"`, `group:"stream_client_interceptor"`),
				),
			),
			fx.Supply(
				fx.Annotate(conf, fx.As(new(LoggingConfig))),
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

func NewGrpcServerInterceptors(logger *zap.Logger, opts ...grpc_zap.Option) (grpc.UnaryServerInterceptor, grpc.StreamServerInterceptor) {
	logOpts := []grpc_zap.Option{
		grpc_zap.WithLevels(DefaultServerCodeToLevel),
	}
	// Append externally supplied options last so they can overwrite our defaults
	logOpts = append(logOpts, opts...)

	return grpc_zap.UnaryServerInterceptor(logger, logOpts...), grpc_zap.StreamServerInterceptor(logger, logOpts...)
}

func NewGrpcClientInterceptors(logger *zap.Logger, opts ...grpc_zap.Option) (grpc.UnaryClientInterceptor, grpc.StreamClientInterceptor) {
	logger = logger.WithOptions(zap.WithCaller(false))
	logOpts := []grpc_zap.Option{
		grpc_zap.WithLevels(DefaultClientCodeToLevel),
	}
	// Append externally supplied options last so they can overwrite our defaults
	logOpts = append(logOpts, opts...)

	return grpc_zap.UnaryClientInterceptor(logger, logOpts...), grpc_zap.StreamClientInterceptor(logger, logOpts...)
}

// NewFxLogger emits an fxevent.Logger that uses the passed in zap logger
// The fxevent.Logger is used to write out the log messages produces by the fx framework
func NewFxLogger(logger *zap.Logger) fxevent.Logger {
	result := &fxevent.ZapLogger{Logger: logger}
	result.UseLogLevel(zapcore.DebugLevel)
	return result
}

// DefaultServerCodeToLevel maps the grpc response code to a logging level
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

// DefaultClientCodeToLevel maps the grpc response code to a logging level
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
