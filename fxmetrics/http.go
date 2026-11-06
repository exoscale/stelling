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
			Buckets: metricBuckets,
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

const metricsMiddlewareWeight = 30

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
			Weight:  metricsMiddlewareWeight,
		},
	}, nil
}

type HttpClientMetrics struct {
	inFlightGauge    prometheus.Gauge
	requestCounter   *prometheus.CounterVec
	requestHistogram *prometheus.HistogramVec
}

func NewHttpClientMetrics() *HttpClientMetrics {
	return &HttpClientMetrics{
		inFlightGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "http_client_inflight_requests",
			Help: "Number of currently ongoing HTTP client requests",
		}),
		requestCounter: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_client_requests_total",
			Help: "Total number of completed HTTP client requests",
		}, []string{"code", "method"}),
		requestHistogram: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_client_request_seconds",
			Help:    "Histogram of response latency (seconds) of HTTP client requests",
			Buckets: metricBuckets,
		}, []string{"code", "method"}),
	}
}

// Describe sends the super-set of all possible descriptors of metrics
// collected by this Collector to the provided channel and returns once
// the last descriptor has been sent.
func (m *HttpClientMetrics) Describe(ch chan<- *prometheus.Desc) {
	m.inFlightGauge.Describe(ch)
	m.requestCounter.Describe(ch)
	m.requestHistogram.Describe(ch)
}

// Collect is called by the Prometheus registry when collecting
// metrics. The implementation sends each collected metric via the
// provided channel and returns once the last metric has been sent.
func (m *HttpClientMetrics) Collect(ch chan<- prometheus.Metric) {
	m.inFlightGauge.Collect(ch)
	m.requestCounter.Collect(ch)
	m.requestHistogram.Collect(ch)
}

type HttpClientMiddlewareResult struct {
	fx.Out

	Middleware *fxhttp.ClientMiddleware `group:"http_client_middleware"`
}

// NewHttpClientMiddleware counts all requests made by the http clients by method and status
func NewHttpClientMiddleware(reg *prometheus.Registry) (HttpClientMiddlewareResult, error) {
	metrics := NewHttpClientMetrics()
	if err := reg.Register(metrics); err != nil {
		return HttpClientMiddlewareResult{}, err
	}

	return HttpClientMiddlewareResult{
		Middleware: &fxhttp.ClientMiddleware{
			RoundTripper: func(next http.RoundTripper) http.RoundTripper {
				return promhttp.InstrumentRoundTripperInFlight(
					metrics.inFlightGauge,
					promhttp.InstrumentRoundTripperCounter(
						metrics.requestCounter,
						promhttp.InstrumentRoundTripperDuration(metrics.requestHistogram, next),
					),
				)
			},
			Weight: metricsMiddlewareWeight,
		},
	}, nil
}
