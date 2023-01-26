// package fxmetrics provides a convenient way to expose prometheus metrics.
package fxmetrics

import (
	"net/http"

	"github.com/exoscale/stelling/fxhttp"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/fx"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
)

// Exposes prometheus metrics.
var Module = fx.Module(
	"metrics",
	fx.Provide(
		fx.Annotate(
			NewMetricsServerConfig,
			fx.ResultTags(`name:"metrics"`),
		),
		NewPrometheusRegistry,
		NewGrpcServerInterceptors,
		NewGrpcClientInterceptors,
	),
	fx.Invoke(
		RegisterMetricsHandlers,
	),
	// Specify last so the server starts after we register the handlers
	fxhttp.NewNamedModule("metrics"),
)

type MetricsConfig interface {
	GetMetrics() *Metrics
}

type Metrics struct {
	fxhttp.Server
	// indicates whether Prometheus grpc middleware exports Histograms or not
	Histograms bool `default:"false"`
	// ProcessName is used as a prefix for certain metrics that can clash
	ProcessName string
}

func (m *Metrics) ApplyDefaults() {
	m.Server.Port = 9091
}

func (m *Metrics) GetMetrics() *Metrics {
	return m
}

func NewMetricsServerConfig(conf MetricsConfig) fxhttp.ServerConfig {
	return &conf.GetMetrics().Server
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

type GrpcServerInterceptorsResult struct {
	fx.Out

	grpc.UnaryServerInterceptor  `group:"unary_server_interceptor"`
	grpc.StreamServerInterceptor `group:"stream_server_interceptor"`
	*grpc_prometheus.ServerMetrics
}

func NewGrpcServerInterceptors(reg *prometheus.Registry, conf MetricsConfig) (GrpcServerInterceptorsResult, error) {
	serverMetrics := grpc_prometheus.NewServerMetrics()
	if err := reg.Register(serverMetrics); err != nil {
		return GrpcServerInterceptorsResult{}, err
	}
	if conf.GetMetrics().Histograms {
		serverMetrics.EnableHandlingTimeHistogram()
	}
	return GrpcServerInterceptorsResult{
		UnaryServerInterceptor:  serverMetrics.UnaryServerInterceptor(),
		StreamServerInterceptor: serverMetrics.StreamServerInterceptor(),
		ServerMetrics:           serverMetrics,
	}, nil
}

func InitializeGrpcServerMetrics(metrics *grpc_prometheus.ServerMetrics, server *grpc.Server) {
	metrics.InitializeMetrics(server)
}

type GrpcClientInterceptorsResult struct {
	fx.Out

	grpc.UnaryClientInterceptor  `group:"unary_client_interceptor"`
	grpc.StreamClientInterceptor `group:"stream_client_interceptor"`
}

func NewGrpcClientInterceptors(reg *prometheus.Registry) (GrpcClientInterceptorsResult, error) {
	clientMetrics := grpc_prometheus.NewClientMetrics()
	if err := reg.Register(clientMetrics); err != nil {
		return GrpcClientInterceptorsResult{}, err
	}
	return GrpcClientInterceptorsResult{
		UnaryClientInterceptor:  clientMetrics.UnaryClientInterceptor(),
		StreamClientInterceptor: clientMetrics.StreamClientInterceptor(),
	}, nil
}

func NewPrometheusRegistry(conf MetricsConfig) (*prometheus.Registry, error) {
	reg := prometheus.NewRegistry()

	if err := reg.Register(collectors.NewGoCollector()); err != nil {
		return nil, err
	}
	opts := collectors.ProcessCollectorOpts{
		Namespace: conf.GetMetrics().ProcessName,
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
