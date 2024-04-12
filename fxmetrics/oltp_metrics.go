package fxmetrics

import (
	"context"
	"time"

	"github.com/exoscale/stelling/fxgrpc"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func NewOltpModule(conf OltpMetricsConfig) fx.Option {
	nameTag := `name:"metrics_oltp"`

	return fx.Options(
		fx.Supply(fx.Annotate(conf, fx.As(new(OltpMetricsConfig)))),
		fx.Provide(
			NewOltpRegistry,
			NewOltpGrpcServerInterceptors,
			NewOltpGrpcClientInterceptors,
		),
	)

	/*
		opts := fx.Options(
			fx.Supply(fx.Annotate(conf, fx.As(new(PushMetricsConfig)))),
			fx.Provide(
				NewPrometheusRegistry,
				NewGrpcServerInterceptors,
				NewGrpcClientInterceptors,
				func(m PushMetricsConfig) MetricsConfig { return m },
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
					),
				)
			}
		}
		return opts
	*/
}

type OltpMetricsConfig interface {
	OltpMetricsConfig() *OltpMetrics
	MetricsConfig() *Metrics
	GrpcClientConfig() *fxgrpc.Client
}

type OltpMetrics struct {
	// Enabled allows oltp metrics support to be toggled on and off
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

func (om *OltpMetrics) OltpMetricsConfig() *OltpMetrics {
	return om
}

func (om *OltpMetrics) MetricsConfig() *Metrics {
	return &Metrics{
		Histograms:  om.Histograms,
		ProcessName: om.ProcessName,
	}
}

func (om *OltpMetrics) GrpcClientConfig() *fxgrpc.Client {
	return &fxgrpc.Client{
		InsecureConnection: om.InsecureConnection,
		CertFile:           om.CertFile,
		KeyFile:            om.KeyFile,
		RootCAFile:         om.RootCAFile,
		Endpoint:           om.Endpoint,
	}
}

func (om *OltpMetrics) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if om == nil {
		return nil
	}

	enc.AddBool("enabled", om.Enabled)
	if om.Enabled {
		enc.AddBool("histograms", om.Histograms)
		enc.AddString("process-name", om.ProcessName)

		enc.AddString("endpoint", om.Endpoint)
		enc.AddBool("insecure-connection", om.InsecureConnection)
		if !om.InsecureConnection {
			enc.AddString("cert-file", om.CertFile)
			enc.AddString("key-file", om.KeyFile)
			enc.AddString("root-ca-file", om.RootCAFile)
		}
	}

	return nil
}

func NewOltpRegistry(lc fx.Lifecycle, conf OltpMetricsConfig, logger *zap.Logger) (metric.MeterProvider, error) {
	oltpConf := conf.OltpMetricsConfig()

	if !oltpConf.Enabled {
		provider := noop.NewMeterProvider()

		return provider, nil
	}

	creds, r, err := fxgrpc.MakeClientTLS(oltpConf, logger)
	if err != nil {
		return nil, err
	}
	if r != nil {
		lc.Append(fx.Hook{OnStart: r.Start, OnStop: r.Stop})
	}

	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(oltpConf.Endpoint),
		otlpmetricgrpc.WithTLSCredentials(creds),
	}

	exporter, err := otlpmetricgrpc.New(context.TODO(), opts...)
	if err != nil {
		return nil, err
	}

	reader := sdkmetric.NewPeriodicReader(
		exporter,
		sdkmetric.WithInterval(oltpConf.PushInterval),
	)

	lc.Append(fx.Hook{OnStop: reader.Shutdown})

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
	)

	return provider, nil
}

type OltpGrpcServerInterceptorParams struct {
	fx.In

	OltpMetricsConfig
	metric.MeterProvider
}

type OltpGrpcServerInterceptorResult struct {
	fx.Out

	*fxgrpc.UnaryServerInterceptor  `group:"unary_server_interceptor"`
	*fxgrpc.StreamServerInterceptor `group:"stream_server_interceptor"`
}

func NewOltpGrpcServerInterceptors(p OltpGrpcServerInterceptorParams) (OltpGrpcServerInterceptorResult, error) {
	opts := []otelgrpc.Option{}
	if p.OltpMetricsConfig.OltpMetricsConfig().Histograms {
		// It looks like this is enabled by default & we can't disable those with otel.
		// TODO: check by comparing the generated metrics
	}

	opts = append(opts, otelgrpc.WithMeterProvider(p.MeterProvider))

	return OltpGrpcServerInterceptorResult{
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

type OltpGrpcClientInterceptorParams struct {
	fx.In

	OltpMetricsConfig
	metric.MeterProvider
}

type OltpGrpcClientInterceptorResult struct {
	fx.Out

	*fxgrpc.UnaryClientInterceptor  `group:"unary_client_interceptor"`
	*fxgrpc.StreamClientInterceptor `group:"stream_client_interceptor"`
}

func NewOltpGrpcClientInterceptors(p OltpGrpcClientInterceptorParams) (OltpGrpcClientInterceptorResult, error) {
	opts := []otelgrpc.Option{}
	if p.OltpMetricsConfig.OltpMetricsConfig().Histograms {
		// It looks like this is enabled by default & we can't disable those with otel.
		// TODO: check by comparing the generated metrics
	}

	opts = append(opts, otelgrpc.WithMeterProvider(p.MeterProvider))

	return OltpGrpcServerInterceptorResult{
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

// func NewOltpGrpcClientInterceptors() {
//
// }

/*
func NewPushModule(conf PushMetricsConfig) fx.Option {
	nameTag := `name:"metrics_pusher"`


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

*/
