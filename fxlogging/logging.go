// Package fxlogging provides a convenient way to create loggers.
package fxlogging

import (
	"context"
	"time"

	"github.com/exoscale/stelling/fxgrpc"
	grpc_logging "github.com/grpc-ecosystem/go-grpc-middleware/logging"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/codes"
)

// NewModule provides a *zap.Logger to the system
// It also provides the following related items:
// * Grpc middleware
// * An adapter to log fx system events
func NewModule(conf LoggingConfig, opts ...Option) fx.Option {
	system := fx.Options(
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
				fx.Annotate(
					NewGrpcClientPayloadInterceptor,
					fx.ParamTags("", `optional:"true"`),
					fx.ResultTags(`group:"unary_client_interceptor"`, `group:"stream_client_interceptor"`),
				),
				fx.Annotate(
					NewGrpcServerPayloadInterceptor,
					fx.ParamTags("", `optional:"true"`),
					fx.ResultTags(`group:"unary_server_interceptor"`, `group:"stream_server_interceptor"`),
				),
			),
			fx.Supply(
				fx.Annotate(conf, fx.As(new(LoggingConfig))),
			),
		),
	)
	for _, opt := range opts {
		system = opt.apply(system)
	}
	return system
}

// An Option configures the logging module
type Option interface {
	apply(fx.Option) fx.Option
}

// optionFunc wraps a func so it satisfies the Option interface.
type optionFunc func(fx.Option) fx.Option

func (f optionFunc) apply(system fx.Option) fx.Option {
	return f(system)
}

// WithClientPayloadLoggingDecider sets a custom decider function for logging request/response payloads on the grpc client
// Should only be specified once
func WithClientPayloadLoggingDecider(decider grpc_logging.ClientPayloadLoggingDecider) Option {
	return optionFunc(func(system fx.Option) fx.Option {
		return fx.Options(
			system,
			fx.Provide(func() grpc_logging.ClientPayloadLoggingDecider { return decider }),
		)
	})
}

// WithServerPayloadLoggingDecider sets a custom decider function for logging request/response payloads on the grpc server
// Should only be specified once
func WithServerPayloadLoggingDecider(decider grpc_logging.ServerPayloadLoggingDecider) Option {
	return optionFunc(func(system fx.Option) fx.Option {
		return fx.Options(
			system,
			fx.Provide(func() grpc_logging.ServerPayloadLoggingDecider { return decider }),
		)
	})
}

// WithZapOpt passes a custom option on to the provided zap logger
// This allows overwriting the default values provided by this module
// Can be supplied multiple times to insert multiple zap.Option into the system
// There is no guarantee to the order in which the options are applied
func WithZapOpt(opt zap.Option) Option {
	return optionFunc(func(system fx.Option) fx.Option {
		return fx.Options(
			system,
			fx.Provide(fx.Annotate(
				func() zap.Option { return opt },
				fx.ResultTags(`group:"zap_opts"`),
			)),
		)
	})
}

// WithGrpcZapClientOpt passes a custom option on to the provided grpc client interceptor
// This allows overwriting the default values provided by this module
// Can be supplied multiple times to insert multiple grpc_zap.Option into the system
// There is no guarantee to the order in which the options are applied
func WithGrpcZapClientOpt(opt grpc_zap.Option) Option {
	return optionFunc(func(system fx.Option) fx.Option {
		return fx.Options(
			system,
			fx.Provide(fx.Annotate(
				func() grpc_zap.Option { return opt },
				fx.ResultTags(`group:"grpc_zap_client_options"`),
			)),
		)
	})
}

// WithGrpcZapServerOpt passes a custom option on to the provided grpc server interceptor
// This allows overwriting the default values provided by this module
// Can be supplied multiple times to insert multiple grpc_zap.Option into the system
// There is no guarantee to the order in which the options are applied
func WithGrpcZapServerOpt(opt grpc_zap.Option) Option {
	return optionFunc(func(system fx.Option) fx.Option {
		return fx.Options(
			system,
			fx.Provide(fx.Annotate(
				func() grpc_zap.Option { return opt },
				fx.ResultTags(`group:"grpc_zap_server_options"`),
			)),
		)
	})
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

const GrpcInterceptorWeight = 50

func NewGrpcServerInterceptors(logger *zap.Logger, opts ...grpc_zap.Option) (*fxgrpc.UnaryServerInterceptor, *fxgrpc.StreamServerInterceptor) {
	logOpts := []grpc_zap.Option{
		grpc_zap.WithLevels(DefaultServerCodeToLevel),
	}
	// Append externally supplied options last so they can overwrite our defaults
	logOpts = append(logOpts, opts...)

	unaryIx := &fxgrpc.UnaryServerInterceptor{Weight: GrpcInterceptorWeight, Interceptor: grpc_zap.UnaryServerInterceptor(logger, logOpts...)}
	streamIx := &fxgrpc.StreamServerInterceptor{Weight: GrpcInterceptorWeight, Interceptor: grpc_zap.StreamServerInterceptor(logger, logOpts...)}
	return unaryIx, streamIx
}

func NewGrpcClientInterceptors(logger *zap.Logger, opts ...grpc_zap.Option) (*fxgrpc.UnaryClientInterceptor, *fxgrpc.StreamClientInterceptor) {
	logger = logger.WithOptions(zap.WithCaller(false))
	logOpts := []grpc_zap.Option{
		grpc_zap.WithLevels(DefaultClientCodeToLevel),
	}
	// Append externally supplied options last so they can overwrite our defaults
	logOpts = append(logOpts, opts...)

	unaryIx := &fxgrpc.UnaryClientInterceptor{Weight: GrpcInterceptorWeight, Interceptor: grpc_zap.UnaryClientInterceptor(logger, logOpts...)}
	streamIx := &fxgrpc.StreamClientInterceptor{Weight: GrpcInterceptorWeight, Interceptor: grpc_zap.StreamClientInterceptor(logger, logOpts...)}
	return unaryIx, streamIx
}

func NewGrpcServerPayloadInterceptor(logger *zap.Logger, decider grpc_logging.ServerPayloadLoggingDecider) (*fxgrpc.UnaryServerInterceptor, *fxgrpc.StreamServerInterceptor) {
	if decider == nil {
		decider = func(context.Context, string, interface{}) bool {
			return false
		}
	}
	unaryIx := &fxgrpc.UnaryServerInterceptor{
		Weight:      GrpcInterceptorWeight + 1,
		Interceptor: grpc_zap.PayloadUnaryServerInterceptor(logger, decider),
	}
	streamIx := &fxgrpc.StreamServerInterceptor{
		Weight:      GrpcInterceptorWeight + 1,
		Interceptor: grpc_zap.PayloadStreamServerInterceptor(logger, decider),
	}
	return unaryIx, streamIx
}

func NewGrpcClientPayloadInterceptor(logger *zap.Logger, decider grpc_logging.ClientPayloadLoggingDecider) (*fxgrpc.UnaryClientInterceptor, *fxgrpc.StreamClientInterceptor) {
	logger = logger.WithOptions(zap.WithCaller(false))
	if decider == nil {
		decider = func(context.Context, string) bool {
			return false
		}
	}
	unaryIx := &fxgrpc.UnaryClientInterceptor{
		Weight:      GrpcInterceptorWeight + 1,
		Interceptor: grpc_zap.PayloadUnaryClientInterceptor(logger, decider),
	}
	streamIx := &fxgrpc.StreamClientInterceptor{
		Weight:      GrpcInterceptorWeight + 1,
		Interceptor: grpc_zap.PayloadStreamClientInterceptor(logger, decider),
	}
	return unaryIx, streamIx
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
