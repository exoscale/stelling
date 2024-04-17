package fxmetrics

import (
	"context"
	"sync"
	"time"

	"github.com/exoscale/stelling/fxgrpc"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	dto "github.com/prometheus/client_model/go"
)

func NewOtlpModule(conf OtlpMetricsConfig) fx.Option {
	return fx.Options(
		fx.Supply(fx.Annotate(conf, fx.As(new(OtlpMetricsConfig)))),
		fx.Provide(
			NewOtlpMeterProvider,
			NewOtlpGrpcServerInterceptors,
			NewOtlpGrpcClientInterceptors,
		),
	)
}

type OtlpMetricsConfig interface {
	OtlpMetricsConfig() *OtlpMetrics
}

type OtlpMetrics struct {
	// Enabled allows otlp metrics support to be toggled on and off
	Enabled bool
	// PushInterval is the frequency with which metrics are pushed
	PushInterval time.Duration `default:"15s"`

	// GrpcClient is the client used to talk to the collector
	GrpcClient *fxgrpc.Client `validate:"required_with=Enabled,omitempty"`
}

func (om *OtlpMetrics) OtlpMetricsConfig() *OtlpMetrics {
	return om
}

func NewOtlpMeterProvider(lc fx.Lifecycle, conf OtlpMetricsConfig, logger *zap.Logger) (metric.MeterProvider, error) {
	otlpConf := conf.OtlpMetricsConfig()

	if !otlpConf.Enabled {
		provider := noop.NewMeterProvider()

		return provider, nil
	}

	creds, r, err := fxgrpc.MakeClientTLS(otlpConf.GrpcClient, logger)
	if err != nil {
		return nil, err
	}
	if r != nil {
		lc.Append(fx.Hook{OnStart: r.Start, OnStop: r.Stop})
	}

	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(otlpConf.GrpcClient.Endpoint),
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

	lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
		return reader.Shutdown(ctx)
	}})

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
	)

	// Attach golang runtime metrics directly
	lc.Append(fx.Hook{OnStart: func(_ context.Context) error {
		return runtime.Start(runtime.WithMeterProvider(provider))
	}})

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
	opts := []otelgrpc.Option{
		otelgrpc.WithMeterProvider(p.MeterProvider),
	}

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
	opts := []otelgrpc.Option{
		otelgrpc.WithMeterProvider(p.MeterProvider),
	}

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

// Helpers

func FromGaugeVec(vec *prometheus.GaugeVec) metric.Float64Callback {
	return func(ctx context.Context, io metric.Float64Observer) error {
		grp := sync.WaitGroup{}
		defer grp.Wait()

		metrics := make(chan prometheus.Metric)
		grp.Add(1)
		go func() {
			defer grp.Done()
			vec.Collect(metrics)
			close(metrics)
		}()

		for mymetric := range metrics {
			protometric := dto.Metric{}
			if err := mymetric.Write(&protometric); err != nil {
				// TODO: take a logger here & display the errors?
				continue
			}

			attributes := []attribute.KeyValue{}
			for _, label := range protometric.Label {
				if label.Name != nil && label.Value != nil {
					attributes = append(attributes,
						attribute.String(*label.Name, *label.Value),
					)
				}
			}

			if value := protometric.Gauge.Value; value != nil {
				io.Observe(*value, metric.WithAttributes(attributes...))
			}
		}

		return nil
	}
}
