package fxtracing

import (
	"context"
	"fmt"
	"net/http"

	"github.com/exoscale/stelling/fxgrpc"
	"github.com/exoscale/stelling/fxhttp"
	"github.com/go-logr/zapr"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/stats"
)

// NewModule provides an opentelemetry TracingProvider to the system
func NewModule(conf TracingConfig) fx.Option {
	return fx.Module(
		"tracing",
		fx.Supply(fx.Annotate(conf, fx.As(new(TracingConfig))), fx.Private),
		fx.Provide(
			NewTracerProvider,
			NewClientStatsHandler,
			NewServerStatsHandler,
			NewHttpMiddleware,
		),
	)
}

type TracingConfig interface {
	TracingConfig() *Tracing
}

type Tracing struct {
	// Enabled allows tracing support to be toggled on and off
	Enabled bool
	// InsecureConnection indicates whether TLS needs to be disabled when connecting to the grpc server
	InsecureConnection bool
	// CertFile is the path to the pem encoded TLS certificate
	CertFile string `validate:"required_if=Enabled true InsecureConnection false,omitempty,file"`
	// KeyFile is the path to the pem encoded private key of the TLS certificate
	KeyFile string `validate:"required_if=Enabled true InsecureConnection false,omitempty,file"`
	// RootCAFile is the  path to a pem encoded CA bundle used to validate server connections
	RootCAFile string `validate:"required_if=Enabled true InsecureConnection false,omitempty,file"`
	// Endpoint is the address + port where the collector can be reached
	Endpoint string `validate:"required_if=Enabled true InsecureConnection false,omitempty,hostname_port"`
}

func (t *Tracing) TracingConfig() *Tracing {
	return t
}

func (t *Tracing) GrpcClientConfig() *fxgrpc.Client {
	return &fxgrpc.Client{
		InsecureConnection: t.InsecureConnection,
		CertFile:           t.CertFile,
		KeyFile:            t.KeyFile,
		RootCAFile:         t.RootCAFile,
		Endpoint:           t.Endpoint,
	}
}

func (t *Tracing) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if t == nil {
		return nil
	}

	enc.AddBool("enabled", t.Enabled)
	if t.Enabled {
		enc.AddString("endpoint", t.Endpoint)
		enc.AddBool("insecure-connection", t.InsecureConnection)
		if !t.InsecureConnection {
			enc.AddString("cert-file", t.CertFile)
			enc.AddString("key-file", t.KeyFile)
			enc.AddString("root-ca-file", t.RootCAFile)
		}
	}

	return nil
}

func NewTracerProvider(lc fx.Lifecycle, conf TracingConfig, logger *zap.Logger) (trace.TracerProvider, error) {
	tracingConf := conf.TracingConfig()
	otel.SetLogger(zapr.NewLogger(logger))

	if !tracingConf.Enabled {
		return noop.NewTracerProvider(), nil
	}

	// If tracing is enabled without an endpoint print traces to stdout
	// This is useful to debug tracing locally, but shouldn't be used in prod
	if tracingConf.Endpoint == "" {
		exporter, err := stdouttrace.New()
		if err != nil {
			return nil, err
		}

		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSyncer(exporter),
		)

		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				return tp.Shutdown(ctx)
			},
		})

		return tp, nil
	}

	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(tracingConf.Endpoint)}

	creds, r, err := fxgrpc.MakeClientTLS(
		tracingConf,
		logger,
	)
	if err != nil {
		return nil, err
	}
	if r != nil {
		lc.Append(fx.Hook{OnStart: r.Start, OnStop: r.Stop})
	}
	opts = append(opts, otlptracegrpc.WithTLSCredentials(creds))

	exporter := otlptracegrpc.NewUnstarted(opts...)

	// TODO: configure sampling here
	// TODO: configure the resource
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
	)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return exporter.Start(ctx)
		},
		OnStop: func(ctx context.Context) error {
			if err := tracerProvider.Shutdown(ctx); err != nil {
				return err
			}
			return exporter.Shutdown(ctx)
		},
	})

	return tracerProvider, nil
}

type ServerStatsHandlerResult struct {
	fx.Out

	Handler stats.Handler `group:"server_stats_handler"`
}

// NewServerStatsHandler returns a grpc stats.Handler for use in a server that automatically traces requests
func NewServerStatsHandler(tracerProvider trace.TracerProvider) ServerStatsHandlerResult {
	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.Baggage{},
		propagation.TraceContext{},
	)

	handler := otelgrpc.NewServerHandler(
		otelgrpc.WithTracerProvider(tracerProvider),
		otelgrpc.WithPropagators(propagator),
	)

	return ServerStatsHandlerResult{
		Handler: handler,
	}
}

type ClientStatsHandlerResult struct {
	fx.Out

	Handler stats.Handler `group:"client_stats_handler"`
}

// NewClientStatsHandler returns a grpc stats.Handler for use in a client that automatically traces requests
func NewClientStatsHandler(tracerProvider trace.TracerProvider) ClientStatsHandlerResult {
	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.Baggage{},
		propagation.TraceContext{},
	)

	handler := otelgrpc.NewClientHandler(
		otelgrpc.WithTracerProvider(tracerProvider),
		otelgrpc.WithPropagators(propagator),
	)

	return ClientStatsHandlerResult{
		Handler: handler,
	}
}

type HttpMiddlewareResult struct {
	fx.Out

	Middleware *fxhttp.Middleware `group:"http_middleware"`
}

func NewHttpMiddleware(tracerProvider trace.TracerProvider) HttpMiddlewareResult {
	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.Baggage{},
		propagation.TraceContext{},
	)

	mw := func(next http.Handler) http.Handler {
		injectedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if rpcMethod := fxhttp.RPCMethodFromContext(ctx); rpcMethod != "" {
				trace.SpanFromContext(ctx).SetAttributes(semconv.RPCMethod(rpcMethod))
			}
			next.ServeHTTP(w, r)
		})

		return otelhttp.NewHandler(
			injectedHandler,
			"",
			otelhttp.WithTracerProvider(tracerProvider),
			otelhttp.WithPropagators(propagator),
			otelhttp.WithSpanNameFormatter(httpSpanNameFormatter),
		)
	}

	return HttpMiddlewareResult{
		Middleware: &fxhttp.Middleware{
			Handler: mw,
			Weight:  20,
		}}
}

func httpSpanNameFormatter(_ string, req *http.Request) string {
	method := req.Method
	if rpcMethod := fxhttp.RPCMethodFromContext(req.Context()); rpcMethod != "" {
		return fmt.Sprintf("%s %s", method, rpcMethod)
	}
	if route := req.Pattern; route != "" {
		return fmt.Sprintf("%s %s", method, route)
	}
	return method
}
