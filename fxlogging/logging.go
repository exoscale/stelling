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

// Provides a logger
var Module = fx.Options(
	fx.WithLogger(NewFxLogger),
	fx.Module(
		"logging",
		fx.Provide(
			NewLogger,
			fx.Annotate(NewGrpcServerInterceptors, fx.ResultTags(`group:"unary_server_interceptor"`, `group:"stream_server_interceptor"`)),
			fx.Annotate(NewGrpcClientInterceptors, fx.ResultTags(`group:"unary_client_interceptor"`, `group:"stream_client_interceptor"`)),
		),
		fx.Supply(ServerCodeToLevel(DefaultServerCodeToLevel)),
		fx.Supply(ClientCodeToLevel(DefaultClientCodeToLevel)),
	),
)

type ServerCodeToLevel grpc_zap.CodeToLevel
type ClientCodeToLevel grpc_zap.CodeToLevel

type LoggingConfig interface {
	GetLogging() *Logging
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

func (l *Logging) GetLogging() *Logging {
	return l
}

func NewLogger(conf LoggingConfig, lc fx.Lifecycle) (*zap.Logger, error) {
	var config zap.Config
	switch conf.GetLogging().Mode {
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
	logger, err := config.Build()
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// By logging here we render the decorated config
			logger.Info("Using configuration", zap.Any("conf", conf))
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

func NewGrpcServerInterceptors(logger *zap.Logger, codeToLevel ServerCodeToLevel) (grpc.UnaryServerInterceptor, grpc.StreamServerInterceptor) {
	logOpts := []grpc_zap.Option{
		grpc_zap.WithLevels(grpc_zap.CodeToLevel(codeToLevel)),
	}

	return grpc_zap.UnaryServerInterceptor(logger, logOpts...), grpc_zap.StreamServerInterceptor(logger, logOpts...)
}

func NewGrpcClientInterceptors(logger *zap.Logger, codeToLevel ClientCodeToLevel) (grpc.UnaryClientInterceptor, grpc.StreamClientInterceptor) {
	logger = logger.WithOptions(zap.WithCaller(false))
	logOpts := []grpc_zap.Option{
		grpc_zap.WithLevels(grpc_zap.CodeToLevel(codeToLevel)),
	}

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
