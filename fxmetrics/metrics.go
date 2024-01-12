// package fxmetrics provides a convenient way to expose prometheus metrics.
package fxmetrics

import (
	"net/http"
	"regexp"

	"github.com/exoscale/stelling/fxgrpc"
	"github.com/exoscale/stelling/fxhttp"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/fx"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
)

// NewModule Exposes prometheus metrics.
func NewModule(conf MetricsConfig) fx.Option {
	return fx.Module(
		"metrics",
		fx.Supply(fx.Annotate(conf, fx.As(new(MetricsConfig)))),
		fx.Provide(
			NewPrometheusRegistry,
			NewGrpcServerInterceptors,
			NewGrpcClientInterceptors,
		),
		fx.Invoke(
			RegisterMetricsHandlers,
		),
		// Specify last so the server starts after we register the handlers
		fxhttp.NewNamedModule("metrics", &conf.MetricsConfig().Server),
	)
}

type MetricsConfig interface {
	MetricsConfig() *Metrics
}

type Metrics struct {
	fxhttp.Server

	// indicates whether Prometheus grpc middleware exports Histograms or not
	Histograms bool `default:"false"`
	// ProcessName is used as a prefix for certain metrics that can clash
	ProcessName string
}

func (m *Metrics) ApplyDefaults() {
	m.Server.Address = ":9091"
}

func (m *Metrics) MetricsConfig() *Metrics {
	return m
}

func (m *Metrics) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if m == nil {
		return nil
	}

	if err := enc.AddObject("server", &m.Server); err != nil {
		return err
	}

	enc.AddBool("histograms", m.Histograms)
	if m.ProcessName != "" {
		enc.AddString("processname", m.ProcessName)
	}
	return nil
}

type RegisterParams struct {
	fx.In

	Reg    *prometheus.Registry
	Server *http.Server `name:"metrics"`
}

func RegisterMetricsHandlers(p RegisterParams) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(p.Reg, promhttp.HandlerOpts{}))
	p.Server.Handler = mux
}

type GrpcServerInterceptorParams struct {
	fx.In

	Conf         MetricsConfig
	Reg          *prometheus.Registry
	HistogramOps []grpc_prometheus.HistogramOption `optional:"true"`
}

type GrpcServerInterceptorsResult struct {
	fx.Out

	*fxgrpc.UnaryServerInterceptor  `group:"unary_server_interceptor"`
	*fxgrpc.StreamServerInterceptor `group:"stream_server_interceptor"`
	*grpc_prometheus.ServerMetrics
}

const GrpcInterceptorWeight = 60

func NewGrpcServerInterceptors(p GrpcServerInterceptorParams) (GrpcServerInterceptorsResult, error) {
	serverMetrics := grpc_prometheus.NewServerMetrics()
	if p.Conf.MetricsConfig().Histograms {
		serverMetrics.EnableHandlingTimeHistogram(p.HistogramOps...)
	}
	if err := p.Reg.Register(serverMetrics); err != nil {
		return GrpcServerInterceptorsResult{}, err
	}

	return GrpcServerInterceptorsResult{
		UnaryServerInterceptor: &fxgrpc.UnaryServerInterceptor{
			Weight:      GrpcInterceptorWeight,
			Interceptor: serverMetrics.UnaryServerInterceptor(),
		},
		StreamServerInterceptor: &fxgrpc.StreamServerInterceptor{
			Weight:      GrpcInterceptorWeight,
			Interceptor: serverMetrics.StreamServerInterceptor(),
		},
		ServerMetrics: serverMetrics,
	}, nil
}

func InitializeGrpcServerMetrics(metrics *grpc_prometheus.ServerMetrics, server *grpc.Server) {
	metrics.InitializeMetrics(server)
}

type GrpcClientInterceptorsResult struct {
	fx.Out

	*fxgrpc.UnaryClientInterceptor  `group:"unary_client_interceptor"`
	*fxgrpc.StreamClientInterceptor `group:"stream_client_interceptor"`
}

func NewGrpcClientInterceptors(reg *prometheus.Registry) (GrpcClientInterceptorsResult, error) {
	clientMetrics := grpc_prometheus.NewClientMetrics()
	if err := reg.Register(clientMetrics); err != nil {
		return GrpcClientInterceptorsResult{}, err
	}
	return GrpcClientInterceptorsResult{
		UnaryClientInterceptor: &fxgrpc.UnaryClientInterceptor{
			Weight:      GrpcInterceptorWeight,
			Interceptor: clientMetrics.UnaryClientInterceptor(),
		},
		StreamClientInterceptor: &fxgrpc.StreamClientInterceptor{
			Weight:      GrpcInterceptorWeight,
			Interceptor: clientMetrics.StreamClientInterceptor(),
		},
	}, nil
}

func NewPrometheusRegistry(conf MetricsConfig) (*prometheus.Registry, error) {
	reg := prometheus.NewRegistry()

	err := reg.Register(
		collectors.NewGoCollector(
			collectors.WithGoCollectorRuntimeMetrics(
				collectors.GoRuntimeMetricsRule{Matcher: regexp.MustCompile("/.*")},
			),
		),
	)
	if err != nil {
		return nil, err
	}
	opts := collectors.ProcessCollectorOpts{
		Namespace: conf.MetricsConfig().ProcessName,
	}
	if err := reg.Register(collectors.NewProcessCollector(opts)); err != nil {
		return nil, err
	}

	// TODO: I expect prometheus to add this to the BuildInfo collector, we can swap
	// over to that one once it happens
	if err := reg.Register(NewVersionCollector()); err != nil {
		return nil, err
	}

	return reg, nil
}
