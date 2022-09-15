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
			NewGrpcServerInterceptors,
			NewGrpcClientInterceptors,
		),
		fx.Supply(DefaultCodeToLevel),
	),
)

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
		OnStop: func(ctx context.Context) error {
			_ = logger.Sync()
			return nil
		},
	})

	logger.Info("Using configuration", zap.Any("conf", conf))

	return logger, nil
}

// ISO8601UTCTimeEncoder is like zapcore.ISO8601TimeEncoder but the
// the timezone to utc first
func ISO8601UTCTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	t = t.UTC()
	type appendTimeEncoder interface {
		AppendTimeLayout(time.Time, string)
	}

	if enc, ok := enc.(appendTimeEncoder); ok {
		enc.AppendTimeLayout(t, "2006-01-02T15:04:05.000Z0700")
		return
	}

	enc.AppendString(t.Format("2006-01-02T15:04:05.000Z0700"))
}

type GrpcServerInterceptorsResult struct {
	fx.Out

	grpc.UnaryServerInterceptor  `group:"unary_server_interceptor"`
	grpc.StreamServerInterceptor `group:"stream_server_interceptor"`
}

func NewGrpcServerInterceptors(logger *zap.Logger, codeToLevel func(codes.Code) zapcore.Level) GrpcServerInterceptorsResult {
	logOpts := []grpc_zap.Option{
		grpc_zap.WithLevels(codeToLevel),
	}

	return GrpcServerInterceptorsResult{
		UnaryServerInterceptor:  grpc_zap.UnaryServerInterceptor(logger, logOpts...),
		StreamServerInterceptor: grpc_zap.StreamServerInterceptor(logger, logOpts...),
	}
}

type GrpcClientInterceptorsResult struct {
	fx.Out

	grpc.UnaryClientInterceptor  `group:"unary_client_interceptor"`
	grpc.StreamClientInterceptor `group:"stream_client_interceptor"`
}

func NewGrpcClientInterceptors(logger *zap.Logger, codeToLevel func(codes.Code) zapcore.Level) GrpcClientInterceptorsResult {
	logger = logger.WithOptions(zap.WithCaller(false))
	logOpts := []grpc_zap.Option{
		grpc_zap.WithLevels(codeToLevel),
	}

	return GrpcClientInterceptorsResult{
		UnaryClientInterceptor:  grpc_zap.UnaryClientInterceptor(logger, logOpts...),
		StreamClientInterceptor: grpc_zap.StreamClientInterceptor(logger, logOpts...),
	}
}

// NewFxLogger emits an fxevent.Logger that uses the passed in zap logger
// The fxevent.Logger is used to write out the log messages produces by the fx framework
func NewFxLogger(logger *zap.Logger) fxevent.Logger {
	return &fxevent.ZapLogger{Logger: logger}
}

// DefaultCodeToLevel maps the grpc response code to a logging level
func DefaultCodeToLevel(code codes.Code) zapcore.Level {
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
