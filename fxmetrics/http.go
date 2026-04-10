package fxmetrics

import (
	"context"
	"net/http"

	"github.com/exoscale/stelling/fxhttp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/fx"
)

type HttpMetrics struct {
	inFlightGauge    prometheus.Gauge
	handledCounter   *prometheus.CounterVec
	handledHistogram *prometheus.HistogramVec
}

func NewHttpMetrics() *HttpMetrics {
	return &HttpMetrics{
		inFlightGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "http_server_inflight_requests",
			Help: "Number of currently ongoing HTTP server requests",
		}),
		handledCounter: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_server_handled_requests_total",
			Help: "Total number of completed HTTP server requests",
		}, []string{"code", "method", "rpc_method"}),
		handledHistogram: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_server_handling_seconds",
			Help:    "Histogram of response latency (seconds) of HTTP server requests",
			Buckets: []float64{0.001, 0.002, 0.004, 0.008, 0.016, 0.032, 0.064, 0.128, 0.256, 0.512, 1.024, 2.048, 4.096, 8.192},
		}, []string{"code", "method", "rpc_method"}),
	}
}

// Describe sends the super-set of all possible descriptors of metrics
// collected by this Collector to the provided channel and returns once
// the last descriptor has been sent.
func (m *HttpMetrics) Describe(ch chan<- *prometheus.Desc) {
	m.inFlightGauge.Describe(ch)
	m.handledCounter.Describe(ch)
	m.handledHistogram.Describe(ch)
}

// Collect is called by the Prometheus registry when collecting
// metrics. The implementation sends each collected metric via the
// provided channel and returns once the last metric has been sent.
func (m *HttpMetrics) Collect(ch chan<- prometheus.Metric) {
	m.inFlightGauge.Collect(ch)
	m.handledCounter.Collect(ch)
	m.handledHistogram.Collect(ch)
}

type HttpMiddlewareResult struct {
	fx.Out

	Middleware *fxhttp.Middleware `group:"http_middleware"`
}

func NewHttpMiddleware(reg *prometheus.Registry) (HttpMiddlewareResult, error) {
	metrics := NewHttpMetrics()
	if err := reg.Register(metrics); err != nil {
		return HttpMiddlewareResult{}, err
	}

	mw := func(next http.Handler) http.Handler {
		counted := promhttp.InstrumentHandlerCounter(
			metrics.handledCounter,
			next,
			promhttp.WithLabelFromCtx("rpc_method", func(ctx context.Context) string {
				return fxhttp.RPCMethodFromContext(ctx)
			}),
		)
		duration := promhttp.InstrumentHandlerDuration(
			metrics.handledHistogram,
			counted,
			promhttp.WithLabelFromCtx("rpc_method", func(ctx context.Context) string {
				return fxhttp.RPCMethodFromContext(ctx)
			}),
		)
		return promhttp.InstrumentHandlerInFlight(
			metrics.inFlightGauge,
			duration,
		)
	}

	return HttpMiddlewareResult{
		Middleware: &fxhttp.Middleware{
			Handler: mw,
			Weight:  30,
		},
	}, nil
}
