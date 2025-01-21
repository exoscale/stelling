package fxmetrics

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"

	reloader "github.com/exoscale/stelling/fxcert-reloader"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewPushModule(conf PushMetricsConfig) fx.Option {
	nameTag := `name:"metrics_pusher"`

	opts := fx.Options(
		fx.Supply(fx.Annotate(conf, fx.As(new(PushMetricsConfig))), fx.Private),
		fx.Provide(
			NewPrometheusRegistry,
			NewGrpcServerInterceptors,
			NewGrpcClientInterceptors,
		),
		fx.Provide(
			func(m PushMetricsConfig) MetricsConfig { return m },
			fx.Private,
		),
	)
	if conf.PushMetricsConfig().Endpoint != "" {
		opts = fx.Options(
			opts,
			fx.Provide(
				fx.Annotate(
					ProvideMetricsPusher,
					fx.ParamTags(``, ``, `name:"metrics_pusher" optional:"true"`),
				),
			),
			fx.Invoke(RegisterPushMetrics),
		)
		if conf.PushMetricsConfig().CertFile != "" {
			opts = fx.Options(
				opts,
				fx.Provide(
					fx.Annotate(
						GetCertReloaderConfig,
						fx.ResultTags(nameTag),
					),
					fx.Annotate(
						reloader.ProvideCertReloader,
						fx.ParamTags(``, nameTag, ``),
						fx.ResultTags(nameTag),
					),
					fx.Private,
				),
			)
		}
	}
	return opts
}

type PushMetricsConfig interface {
	PushMetricsConfig() *PushMetrics
	MetricsConfig() *Metrics
}

type PushMetrics struct {
	// InsecureConnection indicates whether TLS needs to be disabled when connecting to PushGateway
	InsecureConnection bool
	// CertFile is the path to the pem encoded TLS certificate
	CertFile string `validate:"omitempty,file"`
	// KeyFile is the path to the pem encoded private key of the TLS certificate
	KeyFile string `validate:"required_with=CertFile,omitempty,file"`
	// RootCAFile is the path to a pem encoded CA cert bundle used to validate server connections
	RootCAFile string `validate:"omitempty,file"`
	// indicates whether Prometheus grpc middleware exports Histograms or not
	Histograms bool `default:"false"`
	// ProcessName is used as a prefix for certain metrics that can clash
	ProcessName string
	// Endpoint is the URL on which the prometheus pushgateway can be reached
	Endpoint string `validate:"omitempty,url"`
	// JobName is the name of the job in PushGateway
	JobName string `validate:"required_with=Endpoint"`
	// GroupingLabelKey is the label on which PushGateway groups metrics
	// (ie: you can keep a copy of each metric for each value of the GroupingLabelKey)
	GroupingLabelKey string ``
	// The value for this instance of the GroupingLabel (see GroupingLabelKey)
	GroupingLabelValue string `validate:"required_with=GroupingLabelKey"`

	GroupingLabelKeys []string `validate:"excluded_if=GroupingLabelKey"`
	// The value for this instance of the GroupingLabel (see GroupingLabelKey)
	GroupingLabelValues []string `validate:"required_with=GroupingLabelKeys"`

	// PushInterval is the frequency with which metrics are pushed
	// If the PushInterval is set to 0, metrics will only be pushed when the system stops
	PushInterval time.Duration `default:"15s"`
	// ExtraLabels will add each key as a label with the corresponding value in all produced metrics
	ExtraLabels map[string]string
}

func (m *PushMetrics) PushMetricsConfig() *PushMetrics {
	return m
}

func (m *PushMetrics) MetricsConfig() *Metrics {
	return &Metrics{
		Histograms:  m.Histograms,
		ProcessName: m.ProcessName,
	}
}

func (m *PushMetrics) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if m == nil {
		return nil
	}

	enc.AddString("endpoint", m.Endpoint)
	enc.AddDuration("pushinterval", m.PushInterval)
	enc.AddBool("insecureconnection", m.InsecureConnection)
	if !m.InsecureConnection {
		enc.AddString("certfile", m.CertFile)
		enc.AddString("keyfile", m.KeyFile)
		enc.AddString("rootcafile", m.RootCAFile)
	}

	enc.AddBool("histograms", m.Histograms)
	if m.ProcessName != "" {
		enc.AddString("processname", m.ProcessName)
	}
	enc.AddString("jobname", m.JobName)
	if m.GroupingLabelKey != "" {
		enc.AddString("groupinglabel", m.GroupingLabelKey)
	}
	if len(m.GroupingLabelKeys) > 0 {
		for i := 0; i < len(m.GroupingLabelKeys); i++ {
			enc.AddString("groupinglabel", m.GroupingLabelKeys[i])
		}
	}
	return nil
}

func GetCertReloaderConfig(conf PushMetricsConfig) *reloader.CertReloaderConfig {
	return &reloader.CertReloaderConfig{
		CertFile:       conf.PushMetricsConfig().CertFile,
		KeyFile:        conf.PushMetricsConfig().KeyFile,
		ReloadInterval: 10 * time.Second,
	}
}

func httpClient(conf *PushMetrics, reloader *reloader.CertReloader) (*http.Client, error) {
	tlsConf := &tls.Config{
		InsecureSkipVerify: conf.InsecureConnection,
	}
	if reloader != nil {
		tlsConf.GetClientCertificate = reloader.GetClientCertificate
	}
	if conf.RootCAFile != "" {
		certPool, err := x509.SystemCertPool()
		if err != nil {
			return nil, err
		}
		ca, err := os.ReadFile(conf.RootCAFile)
		if err != nil {
			return nil, err
		}
		if ok := certPool.AppendCertsFromPEM(ca); !ok {
			return nil, fmt.Errorf("failed to parse RootCAFile: %s", conf.RootCAFile)
		}
		tlsConf.RootCAs = certPool
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConf}}, nil
}

func ProvideMetricsPusher(lc fx.Lifecycle, conf PushMetricsConfig, reloader *reloader.CertReloader, logger *zap.Logger) (*push.Pusher, error) {
	pConf := conf.PushMetricsConfig()
	logger = logger.Named("metrics-pusher")

	client, err := httpClient(pConf, reloader)
	if err != nil {
		return nil, err
	}
	pusher := push.New(pConf.Endpoint, pConf.JobName).Client(client)

	if pConf.GroupingLabelKey != "" {
		pusher = pusher.Grouping(pConf.GroupingLabelKey, pConf.GroupingLabelValue)
	}
	if len(pConf.GroupingLabelKeys) > 0 {
		for i := 0; i < len(pConf.GroupingLabelKeys); i++ {
			pusher = pusher.Grouping(pConf.GroupingLabelKeys[i], pConf.GroupingLabelValues[i])
		}
	}

	for name, value := range pConf.ExtraLabels {
		pusher = pusher.Grouping(name, value)
	}

	if pConf.PushInterval == 0 {
		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				logger.Debug("Pushing final metrics")
				return pusher.Push()
			},
		})
	} else {
		done := make(chan struct{})

		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				go func() {
					logger.Debug("Starting metrics reporting to pushgateway")
					ticker := time.NewTicker(pConf.PushInterval)
					for {
						select {
						case <-done:
							logger.Debug("Stopping metrics reporting to pushgateway")
							ticker.Stop()
							return
						case <-ticker.C:
							if err := pusher.Add(); err != nil {
								logger.Error("Failed to push metrics", zap.Error(err))
							}
						}
					}
				}()
				return nil
			},
			OnStop: func(ctx context.Context) error {
				close(done)
				logger.Debug("Pushing final metrics")
				return pusher.Add()
			},
		})
	}

	return pusher, nil
}

func RegisterPushMetrics(reg *prometheus.Registry, pusher *push.Pusher) {
	pusher.Gatherer(reg)
}
