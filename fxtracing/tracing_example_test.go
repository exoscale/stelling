package fxtracing_test

import (
	"context"
	"time"

	sconfig "github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/fxlogging"
	"github.com/exoscale/stelling/fxtracing"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Config struct {
	fxlogging.Logging
	fxtracing.Tracing
}

func Example() {
	conf := &Config{}
	args := []string{"tracing-test", "--tracing.enabled", "--tracing.insecure-connection", "--logging.mode", "production"}
	if err := sconfig.Load(conf, args); err != nil {
		panic(err)
	}
	app := fx.New(fx.Options(
		fxlogging.NewModule(conf),
		fxtracing.NewModule(conf),
		// zapOpts contains options to make the logs determistic so we can test the output
		fx.Supply(fx.Annotate(zapOpts, fx.ResultTags(`group:"zap_opts,flatten"`))),
		fx.Invoke(run),
	))

	app.Run()

	// This does print the span as json to stdout: we're not asserting over it because it can't be made deterministic
	// If we disable the timestamp in the stdouttracer and pass in an ID generator that always returns the same ID, it might work
	// But then I also need to figure out why the example test isn't currently checking the output anyway

	// Output:
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Using configuration","conf":{"Mode":"production","Protocol":"grpc","Enabled":true,"InsecureConnection":true,"CertFile":"","KeyFile":"","RootCAFile":"","Endpoint":""}}
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Final configuration","conf":{"Mode":"production","Protocol":"grpc","Enabled":true,"InsecureConnection":true,"CertFile":"","KeyFile":"","RootCAFile":"","Endpoint":""}}
}

func run(lc fx.Lifecycle, sd fx.Shutdowner, tp trace.TracerProvider) {
	// Create a tracer for this particular component
	// It is supposed to be persisted and reused
	tracer := tp.Tracer("demoapp")

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				// Create a new span
				// If a span is already present in the given context, the new span
				// will a child of that one, otherwise it will be a root span
				// The resulting context embeds the new span
				ctx, span := tracer.Start(context.Background(), "my-job")
				job(ctx)
				defer span.End()
				sd.Shutdown() //nolint:errcheck
			}()
			return nil
		},
	})
}

func job(_ context.Context) {
	time.Sleep(1 * time.Millisecond)
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
