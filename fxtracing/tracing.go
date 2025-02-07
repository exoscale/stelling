package fxtracing

import (
	"context"
	"fmt"

	fxcert_reloader "github.com/exoscale/stelling/fxcert-reloader"
	"github.com/exoscale/stelling/fxgrpc"
	"github.com/go-logr/zapr"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewModule provides an opentelemetry TracingProvider to the system
func NewModule(conf TracingConfig) fx.Option {
	return fx.Module(
		"tracing",
		fx.Supply(fx.Annotate(conf, fx.As(new(TracingConfig))), fx.Private),
		fx.Provide(
			NewTracerProvider,
			NewGrpcServerInterceptors,
			NewGrpcClientInterceptors,
		),
	)
}

type TracingConfig interface {
	TracingConfig() *Tracing
}

type Tracing struct {
	Protocol string `default:"grpc" validate:"oneof=grpc http"`
	// Enabled allows tracing support to be toggled on and off
	Enabled bool
	// InsecureConnection indicates whether TLS needs to be disabled when connecting to the grpc server
	InsecureConnection bool
	// CertFile is the path to the pem encoded TLS certificate
	CertFile string `validate:"required_if=Enabled true InsecureConnection false,omitempty,file"`
	// KeyFile is the path to the pem encoded private key of the TLS certificate
	KeyFile string `validate:"required_if=Enabled true InsecureConnection false,omitempty,file"`
	// RootCAFile is the  path to a pem encoded CA bundle used to validate server connections
	RootCAFile string `validate:"required_if=Enabled true InsecureConnection false,omitempty,file"`
	// Endpoint is the address + port where the collector can be reached
	Endpoint string `validate:"required_if=Enabled true InsecureConnection false,omitempty,hostname_port"`
}

func (t *Tracing) TracingConfig() *Tracing {
	return t
}

func (t *Tracing) GrpcClientConfig() *fxgrpc.Client {
	return &fxgrpc.Client{
		InsecureConnection: t.InsecureConnection,
		CertFile:           t.CertFile,
		KeyFile:            t.KeyFile,
		RootCAFile:         t.RootCAFile,
		Endpoint:           t.Endpoint,
	}
}

func (t *Tracing) HttpClientConfig() *fxcert_reloader.Client {
	return &fxcert_reloader.Client{
		InsecureConnection: t.InsecureConnection,
		CertFile:           t.CertFile,
		KeyFile:            t.KeyFile,
		RootCAFile:         t.RootCAFile,
	}
}

func (t *Tracing) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if t == nil {
		return nil
	}

	enc.AddBool("enabled", t.Enabled)
	if t.Enabled {
		enc.AddString("endpoint", t.Endpoint)
		enc.AddBool("insecure-connection", t.InsecureConnection)
		if !t.InsecureConnection {
			enc.AddString("cert-file", t.CertFile)
			enc.AddString("key-file", t.KeyFile)
			enc.AddString("root-ca-file", t.RootCAFile)
		}
	}

	return nil
}

func NewTracerProvider(lc fx.Lifecycle, conf TracingConfig, logger *zap.Logger) (trace.TracerProvider, error) {
	tracingConf := conf.TracingConfig()
	otel.SetLogger(zapr.NewLogger(logger))

	if !tracingConf.Enabled {
		return noop.NewTracerProvider(), nil
	}

	// If tracing is enabled without an endpoint print traces to stdout
	// This is useful to debug tracing locally, but shouldn't be used in prod
	if tracingConf.Endpoint == "" {
		exporter, err := stdouttrace.New()
		if err != nil {
			return nil, err
		}

		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSyncer(exporter),
		)

		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				return tp.Shutdown(ctx)
			},
		})

		return tp, nil
	}

	var exporter *otlptrace.Exporter

	switch tracingConf.Protocol {
	case "grpc":
		creds, r, err := fxgrpc.MakeClientTLS(
			tracingConf,
			logger,
		)
		if err != nil {
			return nil, err
		}
		if r != nil {
			lc.Append(fx.Hook{OnStart: r.Start, OnStop: r.Stop})
		}

		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(tracingConf.Endpoint),
			otlptracegrpc.WithTLSCredentials(creds),
		}

		exporter = otlptracegrpc.NewUnstarted(opts...)
	case "http":
		creds, r, err := fxcert_reloader.MakeClientTLS(tracingConf, logger)
		if err != nil {
			return nil, err
		}
		if r != nil {
			lc.Append(fx.Hook{OnStart: r.Start, OnStop: r.Stop})
		}

		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(tracingConf.Endpoint),
			otlptracehttp.WithTLSClientConfig(creds),
		}

		exporter = otlptracehttp.NewUnstarted(opts...)
	default:
		return nil, fmt.Errorf("Invalid protocol `%v`", tracingConf.Protocol)
	}

	// TODO: configure sampling here
	// TODO: configure the resource
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
	)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return exporter.Start(ctx)
		},
		OnStop: func(ctx context.Context) error {
			if err := tracerProvider.Shutdown(ctx); err != nil {
				return err
			}
			return exporter.Shutdown(ctx)
		},
	})

	return tracerProvider, nil
}

type GrpcServerInterceptorsResult struct {
	fx.Out

	*fxgrpc.UnaryServerInterceptor  `group:"unary_server_interceptor"`
	*fxgrpc.StreamServerInterceptor `group:"stream_server_interceptor"`
}

const GrpcInterceptorWeight = 30

// NewGrpcClientInterceptors returns OpenTelemetry tracing interceptors that can be used as middleware in a gRPC server
func NewGrpcServerInterceptors(tracerProvider trace.TracerProvider) (GrpcServerInterceptorsResult, error) {

	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.Baggage{},
		propagation.TraceContext{},
	)

	// We explicitly rely on the deprecated interceptor implementation
	// The new implementation relies on a stats.Handler which is incompatible
	// with receive buffer reuse: https://github.com/grpc/grpc-go/blob/master/experimental/experimental.go#L40-L42
	// The receive buffer reuse is important for our performance sensitive use cases

	return GrpcServerInterceptorsResult{
		UnaryServerInterceptor: &fxgrpc.UnaryServerInterceptor{
			Weight: GrpcInterceptorWeight,
			Interceptor: otelgrpc.UnaryServerInterceptor( //nolint:staticcheck
				otelgrpc.WithTracerProvider(tracerProvider),
				otelgrpc.WithPropagators(propagator),
			),
		},
		StreamServerInterceptor: &fxgrpc.StreamServerInterceptor{
			Weight: GrpcInterceptorWeight,
			Interceptor: otelgrpc.StreamServerInterceptor( //nolint:staticcheck
				otelgrpc.WithTracerProvider(tracerProvider),
				otelgrpc.WithPropagators(propagator),
			),
		},
	}, nil
}

type GrpcClientInterceptorsResult struct {
	fx.Out

	*fxgrpc.UnaryClientInterceptor  `group:"unary_client_interceptor"`
	*fxgrpc.StreamClientInterceptor `group:"stream_client_interceptor"`
}

// NewGrpcClientInterceptors returns OpenTelemetry tracing interceptors that can be used as middleware in a gRPC client
func NewGrpcClientInterceptors(tracerProvider trace.TracerProvider) (GrpcClientInterceptorsResult, error) {
	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.Baggage{},
		propagation.TraceContext{},
	)

	// We explicitly rely on the deprecated interceptor implementation
	// The new implementation relies on a stats.Handler which is incompatible
	// with receive buffer reuse: https://github.com/grpc/grpc-go/blob/master/experimental/experimental.go#L40-L42
	// The receive buffer reuse is important for our performance sensitive use cases

	return GrpcClientInterceptorsResult{
		UnaryClientInterceptor: &fxgrpc.UnaryClientInterceptor{
			Weight: GrpcInterceptorWeight,
			Interceptor: otelgrpc.UnaryClientInterceptor( //nolint:staticcheck
				otelgrpc.WithTracerProvider(tracerProvider),
				otelgrpc.WithPropagators(propagator),
			),
		},
		StreamClientInterceptor: &fxgrpc.StreamClientInterceptor{
			Weight: GrpcInterceptorWeight,
			Interceptor: otelgrpc.StreamClientInterceptor( //nolint:staticcheck
				otelgrpc.WithTracerProvider(tracerProvider),
				otelgrpc.WithPropagators(propagator),
			),
		},
	}, nil
}
