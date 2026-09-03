package fxlogging_test

import (
	"time"

	sconfig "github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/fxlogging"
	"github.com/exoscale/stelling/fxsentry"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Config struct {
	fxlogging.Logging
	fxsentry.Sentry
	APIKey sconfig.Secret
}

func Example() {
	conf := &Config{}
	args := []string{"logging-test", "--logging.mode", "production", "--api-key", "my-key"}
	if err := sconfig.Load(conf, args); err != nil {
		panic(err)
	}
	app := fx.New(fx.Options(
		fxlogging.NewModule(
			conf,
			// these options make the logs determistic so we can test the output
			// Normal programs will 90% of the time only need the standard module
			// It does however demonstrate how additional zap options can be injected
			fxlogging.WithZapOption(zap.WithCaller(false)),
			fxlogging.WithZapOption(zap.WithClock(&fixedClock{ts: 1257894000})),
		),
		fx.Invoke(run),
	))

	app.Run()

	// Output:
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Using configuration","conf":{"Mode":"production","Dsn":"","Environment":"prod","Version":"","Debug":false,"Process":"","APIKey":"*****"}}
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Example log"}
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Final configuration","conf":{"Mode":"production","Dsn":"","Environment":"prod","Version":"","Debug":false,"Process":"","APIKey":"*****"}}
}

func run(sd fx.Shutdowner, logger *zap.Logger) {
	logger.Info("Example log")
	sd.Shutdown() //nolint:errcheck
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
