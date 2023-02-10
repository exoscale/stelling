package fxmetrics_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	sconfig "github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/fxlogging"
	"github.com/exoscale/stelling/fxmetrics"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Config struct {
	fxlogging.Logging
	fxmetrics.Metrics
}

func Example() {
	conf := &Config{}
	args := []string{"metrics-test", "--logging.mode", "production"}
	if err := sconfig.Load(conf, args); err != nil {
		panic(err)
	}
	opts := fx.Options(
		fxlogging.NewModule(conf),
		fxmetrics.NewModule(conf),
		// zapOpts contains options to make the logs determistic so we can test the output
		fx.Supply(fx.Annotate(zapOpts, fx.ResultTags(`group:"zap_opts,flatten"`))),
		fx.Provide(provideMetrics),
		fx.Invoke(registerMetrics),
		fx.Invoke(run),
	)
	if err := fx.ValidateApp(opts); err != nil {
		panic(err)
	}

	fx.New(opts).Run()

	// Output:
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Using configuration","conf":{"Mode":"production","Port":9091,"TLS":false,"CertFile":"","KeyFile":"","ClientCAFile":"","Histograms":false,"ProcessName":""}}
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Final configuration","conf":{"Mode":"production","Port":9091,"TLS":false,"CertFile":"","KeyFile":"","ClientCAFile":"","Histograms":false,"ProcessName":""}}
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Starting http server","port":9091}
	// Response code for GET http://localhost:9091/metrics 200
	// Payload contains the custom metric true
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Stopping http server"}
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Done serving http"}
}

func provideMetrics() prometheus.Collector {
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "app",
		Subsystem: "component",
		Name:      "example_total",
		Help:      "Total number of times run was called",
	}, []string{})
	// Metrics are provisioned lazily, by reading it here we ensure it's present
	counter.GetMetricWithLabelValues() //nolint:errcheck
	return counter
}

func registerMetrics(metric prometheus.Collector, reg *prometheus.Registry) error {
	return reg.Register(metric)
}

func run(lc fx.Lifecycle, sd fx.Shutdowner) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				// By default the metrics server binds to 0.0.0.0:9091
				resp, err := http.DefaultClient.Get("http://localhost:9091/metrics")
				if err != nil {
					panic(err)
				}
				fmt.Println("Response code for GET http://localhost:9091/metrics", resp.StatusCode)
				body := resp.Body
				metrics, err := io.ReadAll(body)
				if err != nil {
					panic(err)
				}
				hasMetric := bytes.Contains(metrics, []byte("app_component_example_total 0"))
				fmt.Println("Payload contains the custom metric", hasMetric)
				defer body.Close()
				sd.Shutdown() //nolint:errcheck
			}()
			return nil
		},
	})
}

var zapOpts = []zap.Option{
	zap.WithCaller(false),
	zap.WithClock(&fixedClock{ts: 1257894000}),
}

type fixedClock struct {
	ts int64
}

func (c *fixedClock) Now() time.Time {
	return time.Unix(c.ts, 0).UTC()
}

func (c *fixedClock) NewTicker(d time.Duration) *time.Ticker {
	return time.NewTicker(d)
}
