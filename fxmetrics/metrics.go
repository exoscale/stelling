package fxmetrics

import (
	"context"
	"fmt"
	"net/http"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
)

var Module = fx.Provide(
	NewRegistry,
	NewGrpcServerInterceptors,
	NewGrpcClientInterceptors,
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
	}

	return nil
}

func NewRegistry(lc fx.Lifecycle, conf MetricsConfig, logger *zap.Logger) *prometheus.Registry {
	reg := prometheus.NewRegistry()
	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", conf.GetMetrics().Port),
		Handler: mux,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// TODO: use the zap logger
			mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

			logger.Info("Starting metrics server", zap.Int("port", conf.GetMetrics().Port))
			if conf.GetMetrics().TLS {
				go func() {
					if err := server.ListenAndServeTLS(conf.GetMetrics().CertFile, conf.GetMetrics().KeyFile); err != http.ErrServerClosed {
						logger.Fatal("Error while serving metrics", zap.Error(err))
					} else {
						logger.Info("Done serving metrics")
					}
				}()
			} else {
				go func() {
					if err := server.ListenAndServe(); err != http.ErrServerClosed {
						logger.Fatal("Error while serving metrics", zap.Error(err))
					} else {
						logger.Info("Done serving metrics")
					}
				}()
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping metrics server")
			return server.Shutdown(ctx)
		},
	})

	return reg
}

type GrpcServerInterceptorsResult struct {
	fx.Out

	grpc.UnaryServerInterceptor  `group:"unary_server_interceptor"`
	grpc.StreamServerInterceptor `group:"stream_server_interceptor"`
	*grpc_prometheus.ServerMetrics
}

func NewGrpcServerInterceptors(reg *prometheus.Registry) (GrpcServerInterceptorsResult, error) {
	serverMetrics := grpc_prometheus.NewServerMetrics()
	if err := reg.Register(serverMetrics); err != nil {
		return GrpcServerInterceptorsResult{}, err
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
