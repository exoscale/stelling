package fxlogging_test

import (
	"time"

	sconfig "github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/fxlogging"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Config struct {
	fxlogging.Logging
}

func Example() {
	conf := &Config{}
	args := []string{"logging-test", "--logging.mode", "production"}
	if err := sconfig.Load(conf, args); err != nil {
		panic(err)
	}
	app := fx.New(fx.Options(
		fxlogging.NewModule(conf),
		// zapOpts contains options to make the logs determistic so we can test the output
		// Normal programs will 90% of the time only need the standard module
		// It does however demonstrate how additional zap options can be injected
		fx.Supply(fx.Annotate(zapOpts, fx.ResultTags(`group:"zap_opts,flatten"`))),
		fx.Invoke(run),
	))

	app.Run()

	// Output:
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Using configuration","conf":{"mode":"production"}}
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Example log"}
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Final configuration","conf":{"mode":"production"}}
}

func run(sd fx.Shutdowner, logger *zap.Logger) {
	logger.Info("Example log")
	sd.Shutdown() //nolint:errcheck
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
