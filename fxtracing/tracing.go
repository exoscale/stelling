package fxtracing

import (
	"context"

	"github.com/exoscale/stelling/fxgrpc"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
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
	// SpanLimits allows overwriting the default span limits of the tracing provider
	SpanLimits sdktrace.SpanLimits
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
		enc.AddBool("insecure-connection", t.InsecureConnection)
		if !t.InsecureConnection {
			enc.AddString("cert-file", t.CertFile)
			enc.AddString("key-file", t.KeyFile)
			enc.AddString("root-ca-file", t.RootCAFile)
		}
	}
	empty := sdktrace.SpanLimits{}
	if t.SpanLimits != empty {
		if err := enc.AddReflected("span-limits", t.SpanLimits); err != nil {
			return err
		}
	}

	return nil
}

func NewTracerProvider(lc fx.Lifecycle, conf TracingConfig, logger *zap.Logger) (*sdktrace.TracerProvider, error) {
	tracingConf := conf.GetTracing()

	if !tracingConf.Enabled {
		return nil, nil
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

	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(tracingConf.Endpoint)}

	if tracingConf.InsecureConnection {
		opts = append(opts, otlptracegrpc.WithInsecure())
	} else {
		clientConf := &fxgrpc.Client{
			CertFile:   tracingConf.CertFile,
			KeyFile:    tracingConf.KeyFile,
			RootCAFile: tracingConf.RootCAFile,
		}
		creds, r, err := fxgrpc.MakeClientTLS(
			clientConf,
			logger,
		)
		if err != nil {
			return nil, err
		}
		if r != nil {
			lc.Append(fx.Hook{OnStart: r.Start, OnStop: r.Stop})
		}
		opts = append(opts, otlptracegrpc.WithTLSCredentials(creds))
	}
	exporter := otlptracegrpc.NewUnstarted(opts...)

	// TODO: configure sampling here
	// TODO: configure the resource
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSpanLimits(conf.GetTracing().SpanLimits),
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

	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.Baggage{},
		propagation.TraceContext{},
	)

	return GrpcServerInterceptorsResult{
		UnaryServerInterceptor: otelgrpc.UnaryServerInterceptor(
			otelgrpc.WithTracerProvider(p.TracerProvider),
			otelgrpc.WithPropagators(propagator),
		),
		StreamServerInterceptor: otelgrpc.StreamServerInterceptor(
			otelgrpc.WithTracerProvider(p.TracerProvider),
			otelgrpc.WithPropagators(propagator),
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

	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.Baggage{},
		propagation.TraceContext{},
	)

	return GrpcClientInterceptorsResult{
		UnaryClientInterceptor: otelgrpc.UnaryClientInterceptor(
			otelgrpc.WithTracerProvider(p.TracerProvider),
			otelgrpc.WithPropagators(propagator),
		),
		StreamClientInterceptor: otelgrpc.StreamClientInterceptor(
			otelgrpc.WithTracerProvider(p.TracerProvider),
			otelgrpc.WithPropagators(propagator),
		),
	}, nil
}
