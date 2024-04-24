package fxmetrics

import (
	"context"
	"time"

	"github.com/exoscale/stelling/fxgrpc"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/fx"
	"go.uber.org/zap"

	pBridge "go.opentelemetry.io/contrib/bridges/prometheus"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func NewOtlpModule(conf OtlpMetricsConfig) fx.Option {
	return fx.Options(
		fx.Supply(fx.Annotate(conf, fx.As(new(OtlpMetricsConfig)))),
		fx.Supply(fx.Annotate(conf, fx.As(new(MetricsConfig)))),
		fx.Provide(
			NewPrometheusRegistry,
			NewOtlpMeterProvider,
			NewGrpcServerInterceptors,
			NewGrpcClientInterceptors,
		),
		fx.Invoke(InvokeOtlpMeterProvider),
	)
}

type OtlpMetricsConfig interface {
	OtlpMetricsConfig() *OtlpMetrics
	MetricsConfig() *Metrics
}

type OtlpMetrics struct {
	// Enabled allows otlp metrics support to be toggled on and off
	Enabled bool
	// PushInterval is the frequency with which metrics are pushed
	PushInterval time.Duration `default:"15s"`
	// indicates whether Prometheus grpc middleware exports Histograms or not
	Histograms bool `default:"false"`
	// ProcessName is used as a prefix for certain metrics that can clash
	ProcessName string

	// GrpcClient is the client used to talk to the collector
	GrpcClient fxgrpc.Client `validate:"required_with=Enabled,omitempty"`
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

func NewOtlpMeterProvider(lc fx.Lifecycle, conf OtlpMetricsConfig, reg *prometheus.Registry, logger *zap.Logger) (metric.MeterProvider, error) {
	otlpConf := conf.OtlpMetricsConfig()

	if !otlpConf.Enabled {
		return noop.NewMeterProvider(), nil
	}

	bridge := pBridge.NewMetricProducer(pBridge.WithGatherer(reg))

	creds, r, err := fxgrpc.MakeClientTLS(&otlpConf.GrpcClient, logger)
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
		sdkmetric.WithProducer(bridge),
	)
	// Without a metric provider the reader does not seem to actually do anything
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
		logger.Info("Flushing metrics")
		// This also flushes the embedded reader
		return mp.Shutdown(ctx)
	}})

	return mp, nil
}

// InvokeOtlpMeterProvider can be embedded in a system to ensure the metric.MeterProvider is created
func InvokeOtlpMeterProvider(lc fx.Lifecycle, mp metric.MeterProvider) {}
