package fxlogging

import (
	"context"
	"fmt"

	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var Module = fx.Provide(
	NewLogger,
	NewGrpcServerInterceptors,
	NewGrpcClientIncterceptors,
)

type LoggingConfig interface {
	GetLogging() *Logging
}

// Logging contains the configuration options for the logging module
type Logging struct {
	// LogMode is the preset logging configuration
	Mode string `default:"development" validate:"oneof=production development"`
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
	var logger *zap.Logger
	var err error
	if conf.GetLogging().Mode == "production" {
		logger, err = zap.NewProduction()
	} else {
		logger, err = zap.NewDevelopment()
	}
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

type GrpcServerInterceptorsResult struct {
	fx.Out

	grpc.UnaryServerInterceptor  `group:"unary_server_interceptor"`
	grpc.StreamServerInterceptor `group:"stream_server_interceptor"`
}

func NewGrpcServerInterceptors(logger *zap.Logger) GrpcServerInterceptorsResult {
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

func NewGrpcClientIncterceptors(logger *zap.Logger) GrpcClientInterceptorsResult {
	logOpts := []grpc_zap.Option{
		grpc_zap.WithLevels(codeToLevel),
	}

	return GrpcClientInterceptorsResult{
		UnaryClientInterceptor:  grpc_zap.UnaryClientInterceptor(logger, logOpts...),
		StreamClientInterceptor: grpc_zap.StreamClientInterceptor(logger, logOpts...),
	}
}

// LogAdapter implements fx.Printer on a zap.Logger
type LogAdapter struct {
	*zap.Logger
}

func (a *LogAdapter) Printf(format string, args ...interface{}) {
	a.Debug(fmt.Sprintf(format, args...))
}

func NewFxPrinter(conf LoggingConfig) *LogAdapter {
	var logger *zap.Logger
	if conf.GetLogging().Mode == "production" {
		logger, _ = zap.NewProduction()
	} else {
		logger, _ = zap.NewDevelopment()
	}
	return &LogAdapter{Logger: logger}
}

// codeToLevel maps the grpc response code to a logging level
func codeToLevel(code codes.Code) zapcore.Level {
	switch code {
	case codes.OK:
		return zap.DebugLevel
	case codes.Canceled:
		return zap.DebugLevel
	case codes.Unknown:
		return zap.ErrorLevel
	case codes.InvalidArgument:
		return zap.DebugLevel
	case codes.DeadlineExceeded:
		return zap.WarnLevel
	case codes.NotFound:
		return zap.DebugLevel
	case codes.AlreadyExists:
		return zap.DebugLevel
	case codes.PermissionDenied:
		return zap.WarnLevel
	case codes.Unauthenticated:
		return zap.DebugLevel // unauthenticated requests can happen
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
