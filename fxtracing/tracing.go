package fxtracing

import (
	"context"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/exporters/otlp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpgrpc"
	"go.opentelemetry.io/otel/exporters/stdout"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/fx"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var Module = fx.Provide(
	NewTracerProvider,
	NewGrpcServerInterceptors,
	NewGrpcClientInterceptors,
)

type TracingConfig interface {
	GetTracing() *Tracing
}

type Tracing struct {
	// Enabled allows tracing support to be toggled on and off
	Enabled bool
	// Endpoint is the address + port where the collector can be reached
	Endpoint string `validate:"required_if=TLS true,omitempty,hostname_port"`
	// TLS indicates whether TLS is used to connect to the traces collector
	TLS bool
	// CertFile is the path to the pem encoded TLS certificate
	CertFile string `validate:"required_if=TLS true,omitempty,file"`
	// KeyFile is the path to the pem encoded private key of the TLS certificate
	KeyFile string `validate:"required_if=TLS true,omitempty,file"`
}

func (t *Tracing) GetTracing() *Tracing {
	return t
}

func (t *Tracing) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if t == nil {
		return nil
	}

	enc.AddBool("enabled", t.Enabled)
	if t.Enabled {
		enc.AddString("endpoint", t.Endpoint)
		enc.AddBool("tls", t.TLS)
		if t.TLS {
			enc.AddString("cert-file", t.CertFile)
			enc.AddString("key-file", t.KeyFile)
		}
	}

	return nil
}

func NewTracerProvider(conf TracingConfig, lc fx.Lifecycle) (*sdktrace.TracerProvider, error) {
	tracingConf := conf.GetTracing()

	if !tracingConf.Enabled {
		return nil, nil
	}

	// If tracing is enabled without an endpoint print traces to stdout
	// This is useful to debug tracing locally, but shouldn't be used in prod
	if tracingConf.Endpoint == "" {
		exporter, err := stdout.NewExporter()
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

	opts := []otlpgrpc.Option{otlpgrpc.WithEndpoint(tracingConf.Endpoint)}

	if tracingConf.TLS {
		creds, err := credentials.NewServerTLSFromFile(tracingConf.CertFile, tracingConf.KeyFile)
		if err != nil {
			return nil, err
		}
		opts = append(opts, otlpgrpc.WithTLSCredentials(creds))
	} else {
		opts = append(opts, otlpgrpc.WithInsecure())
	}
	td := otlpgrpc.NewDriver(opts...)

	exporter := otlp.NewUnstartedExporter(td)

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

type GrpcServerInterceptorsParams struct {
	fx.In

	*sdktrace.TracerProvider `optional:"true"`
}

type GrpcServerInterceptorsResult struct {
	fx.Out

	grpc.UnaryServerInterceptor  `group:"unary_server_interceptor"`
	grpc.StreamServerInterceptor `group:"stream_server_interceptor"`
}

// NewGrpcClientInterceptors returns OpenTelemetry tracing interceptors that can be used as middleware in a gRPC server
func NewGrpcServerInterceptors(p GrpcServerInterceptorsParams) (GrpcServerInterceptorsResult, error) {
	if p.TracerProvider == nil {
		return GrpcServerInterceptorsResult{}, nil
	}

	return GrpcServerInterceptorsResult{
		UnaryServerInterceptor: otelgrpc.UnaryServerInterceptor(
			otelgrpc.WithTracerProvider(p.TracerProvider),
		),
		StreamServerInterceptor: otelgrpc.StreamServerInterceptor(
			otelgrpc.WithTracerProvider(p.TracerProvider),
		),
	}, nil
}

type GrpcClientInterceptorsParams struct {
	fx.In

	*sdktrace.TracerProvider `optional:"true"`
}

type GrpcClientInterceptorsResult struct {
	fx.Out

	grpc.UnaryClientInterceptor  `group:"unary_client_interceptor"`
	grpc.StreamClientInterceptor `group:"stream_client_interceptor"`
}

// NewGrpcClientInterceptors returns OpenTelemetry tracing interceptors that can be used as middleware in a gRPC client
func NewGrpcClientInterceptors(p GrpcClientInterceptorsParams) (GrpcClientInterceptorsResult, error) {
	if p.TracerProvider == nil {
		return GrpcClientInterceptorsResult{}, nil
	}

	return GrpcClientInterceptorsResult{
		UnaryClientInterceptor: otelgrpc.UnaryClientInterceptor(
			otelgrpc.WithTracerProvider(p.TracerProvider),
		),
		StreamClientInterceptor: otelgrpc.StreamClientInterceptor(
			otelgrpc.WithTracerProvider(p.TracerProvider),
		),
	}, nil
}
