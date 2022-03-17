//package fxmetrics provides a convenient way to expose prometheus metrics.
package fxmetrics

import (
	"net/http"

	"github.com/exoscale/stelling/fxhttp"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
)

// Exposes prometheus metrics.
var Module = fx.Module(
	"metrics",
	fx.Provide(
		NewPrometheusRegistry,
		NewGrpcServerInterceptors,
		NewGrpcClientInterceptors,
		NewMetricsHttpServer,
	),
	fx.Invoke(
		RegisterMetricsHandlers,
	),
)

type MetricsConfig interface {
	GetMetrics() *Metrics
}

type Metrics struct {
	// Port is the port the Prometheus endpoint will bind to
	Port int `default:"9091" validate:"port"`
	// TLS indicates whether the Prometheus endpoint exposes with TLS
	TLS bool
	// CertFile is the path to the pem encoded TLS certificate
	CertFile string `validate:"required_if=TLS true,omitempty,file"`
	// KeyFile is the path to the pem encoded private key of the TLS certificate
	KeyFile string `validate:"required_if=TLS true,omitempty,file"`
	// ClientCAFile is the path to a pem encoded CA cert bundle used to validate clients
	ClientCAFile string `validate:"excluded_without=TLS,omitempty,file"`
	// indicates whether Prometheus server export Histograms or not
	Histograms bool `default:"false"`
	// ProcessName is used as a prefix for certain metrics that can clash
	ProcessName string
}

func (m *Metrics) GetMetrics() *Metrics {
	return m
}

func (m *Metrics) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if m == nil {
		return nil
	}

	enc.AddInt("port", m.Port)
	enc.AddBool("tls", m.TLS)

	if m.TLS {
		enc.AddString("cert-file", m.CertFile)
		enc.AddString("key-file", m.KeyFile)
		enc.AddString("client-ca-file", m.ClientCAFile)
	}
	enc.AddBool("histograms", m.Histograms)
	return nil
}

type MetricsHttpServerResult struct {
	fx.Out

	Server *http.Server `name:"metrics_server"`
}

func NewMetricsHttpServer(lc fx.Lifecycle, conf MetricsConfig, logger *zap.Logger) (MetricsHttpServerResult, error) {
	sconf := &fxhttp.Server{
		TLS:          conf.GetMetrics().TLS,
		CertFile:     conf.GetMetrics().CertFile,
		KeyFile:      conf.GetMetrics().KeyFile,
		ClientCAFile: conf.GetMetrics().ClientCAFile,
		Port:         conf.GetMetrics().Port,
	}
	server, err := fxhttp.NewHTTPServer(lc, sconf, logger)
	if err != nil {
		return MetricsHttpServerResult{}, err
	}
	return MetricsHttpServerResult{
		Server: server,
	}, nil
}

type RegisterParams struct {
	fx.In

	Reg    *prometheus.Registry
	Server *http.Server `name:"metrics_server"`
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

	// TODO: once we are on go 1.18 the buildinfo exporter will become useful too

	return reg, nil
}
