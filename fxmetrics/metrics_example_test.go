package fxmetrics_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	sconfig "github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/fxmetrics"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

type Config struct {
	fxmetrics.Metrics
}

func Example() {
	conf := &Config{}
	args := []string{"metrics-test", "--metrics.server.address", "localhost:9091"}
	if err := sconfig.Load(conf, args); err != nil {
		panic(err)
	}
	opts := fx.Options(
		// Suppressing fx logs to ensure deterministic output
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		fxmetrics.NewModule(conf),
		fx.Provide(
			zap.NewNop,
			provideMetrics,
		),
		fx.Invoke(registerMetrics),
		fx.Invoke(run),
	)
	if err := fx.ValidateApp(opts); err != nil {
		panic(err)
	}

	fx.New(opts).Run()

	// Output:
	// Response code for GET http://localhost:9091/metrics 200
	// Payload contains the custom metric true
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
				resp, err := http.DefaultClient.Get("http://localhost:9091/metrics") //nolint:noctx
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
