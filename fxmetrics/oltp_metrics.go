package fxmetrics

import (
	"context"
	"time"

	"github.com/exoscale/stelling/fxgrpc"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func NewOtlpModule(conf OtlpMetricsConfig) fx.Option {
	return fx.Options(
		fx.Supply(fx.Annotate(conf, fx.As(new(OtlpMetricsConfig)))),
		fx.Provide(
			NewOtlpMeterProvider,
			NewOtlpGrpcServerInterceptors,
			NewOtlpGrpcClientInterceptors,
			func(conf OtlpMetricsConfig) MetricsConfig { return conf },
		),
	)
}

type OtlpMetricsConfig interface {
	OtlpMetricsConfig() *OtlpMetrics
	MetricsConfig() *Metrics
}

type OtlpMetrics struct {
	// Enabled allows otlp metrics support to be toggled on and off
	Enabled bool

	// indicates whether grpc metrics middleware exports Histograms or not
	Histograms bool `default:"false"`
	// ProcessName is used as a prefix for certain metrics that can clash
	ProcessName string

	// InsecureConnection indicates whether TLS needs to be disabled
	InsecureConnection bool
	// CertFile is the path to the pem encoded TLS certificate
	CertFile string `validate:"required_if=Enabled true InsecureConnection false,omitempty,file"`
	// KeyFile is the path to the pem encoded private key of the TLS certificate
	KeyFile string `validate:"required_if=Enabled true InsecureConnection false,omitempty,file"`
	// RootCAFile is the  path to a pem encoded CA bundle used to validate connections
	RootCAFile string `validate:"required_if=Enabled true InsecureConnection false,omitempty,file"`
	// Endpoint is the address + port where the collector can be reached
	Endpoint string `validate:"required_if=Enabled true InsecureConnection false,omitempty,hostname_port"`

	// PushInterval is the frequency with which metrics are pushed
	PushInterval time.Duration `default:"15s"`
}

func (om *OtlpMetrics) OtlpMetricsConfig() *OtlpMetrics {
	return om
}

func (om *OtlpMetrics) MetricsConfig() *Metrics {
	return &Metrics{
		Histograms:  om.Histograms,
		ProcessName: om.ProcessName,
	}
}

func (om *OtlpMetrics) GrpcClientConfig() *fxgrpc.Client {
	return &fxgrpc.Client{
		InsecureConnection: om.InsecureConnection,
		CertFile:           om.CertFile,
		KeyFile:            om.KeyFile,
		RootCAFile:         om.RootCAFile,
		Endpoint:           om.Endpoint,
	}
}

func NewOtlpMeterProvider(lc fx.Lifecycle, conf OtlpMetricsConfig, logger *zap.Logger) (metric.MeterProvider, error) {
	otlpConf := conf.OtlpMetricsConfig()

	if !otlpConf.Enabled {
		provider := noop.NewMeterProvider()

		return provider, nil
	}

	creds, r, err := fxgrpc.MakeClientTLS(otlpConf, logger)
	if err != nil {
		return nil, err
	}
	if r != nil {
		lc.Append(fx.Hook{OnStart: r.Start, OnStop: r.Stop})
	}

	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(otlpConf.Endpoint),
		otlpmetricgrpc.WithTLSCredentials(creds),
	}

	exporter, err := otlpmetricgrpc.New(context.TODO(), opts...)
	if err != nil {
		return nil, err
	}

	reader := sdkmetric.NewPeriodicReader(
		exporter,
		sdkmetric.WithInterval(otlpConf.PushInterval),
	)

	lc.Append(fx.Hook{OnStop: reader.Shutdown})

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
	)

	return provider, nil
}

type OtlpGrpcServerInterceptorParams struct {
	fx.In

	OtlpMetricsConfig
	metric.MeterProvider
}

type OtlpGrpcServerInterceptorResult struct {
	fx.Out

	*fxgrpc.UnaryServerInterceptor  `group:"unary_server_interceptor"`
	*fxgrpc.StreamServerInterceptor `group:"stream_server_interceptor"`
}

func NewOtlpGrpcServerInterceptors(p OtlpGrpcServerInterceptorParams) (OtlpGrpcServerInterceptorResult, error) {
	opts := []otelgrpc.Option{}

	// Not checking `p.OtlpMetricsConfig.OtlpMetricsConfig().Histograms`, the histograms are always enabled with this SDK

	opts = append(opts, otelgrpc.WithMeterProvider(p.MeterProvider))

	return OtlpGrpcServerInterceptorResult{
		UnaryServerInterceptor: &fxgrpc.UnaryServerInterceptor{
			Weight:      GrpcInterceptorWeight,
			Interceptor: otelgrpc.UnaryServerInterceptor(opts...), //nolint:staticcheck
		},
		StreamServerInterceptor: &fxgrpc.StreamServerInterceptor{
			Weight:      GrpcInterceptorWeight,
			Interceptor: otelgrpc.StreamServerInterceptor(opts...), //nolint:staticcheck
		},
	}, nil
}

type OtlpGrpcClientInterceptorParams struct {
	fx.In

	OtlpMetricsConfig
	metric.MeterProvider
}

type OtlpGrpcClientInterceptorResult struct {
	fx.Out

	*fxgrpc.UnaryClientInterceptor  `group:"unary_client_interceptor"`
	*fxgrpc.StreamClientInterceptor `group:"stream_client_interceptor"`
}

func NewOtlpGrpcClientInterceptors(p OtlpGrpcClientInterceptorParams) (OtlpGrpcClientInterceptorResult, error) {
	opts := []otelgrpc.Option{}
	// Not checking `p.OtlpMetricsConfig.OtlpMetricsConfig().Histograms`, the histograms are always enabled with this SDK

	opts = append(opts, otelgrpc.WithMeterProvider(p.MeterProvider))

	return OtlpGrpcClientInterceptorResult{
		UnaryClientInterceptor: &fxgrpc.UnaryClientInterceptor{
			Weight:      GrpcInterceptorWeight,
			Interceptor: otelgrpc.UnaryClientInterceptor(opts...), //nolint:staticcheck
		},
		StreamClientInterceptor: &fxgrpc.StreamClientInterceptor{
			Weight:      GrpcInterceptorWeight,
			Interceptor: otelgrpc.StreamClientInterceptor(opts...), //nolint:staticcheck
		},
	}, nil
}
