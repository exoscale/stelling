package fxsentry_test

import (
	"fmt"
	"time"

	sconfig "github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/fxlogging"
	"github.com/exoscale/stelling/fxsentry"
	sentry "github.com/getsentry/sentry-go"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Config struct {
	fxlogging.Logging
	fxsentry.Sentry
}

func Example() {
	conf := &Config{}
	args := []string{"sentry-test", "--logging.mode", "production"}
	if err := sconfig.Load(conf, args); err != nil {
		panic(err)
	}
	app := fx.New(fx.Options(
		fxlogging.NewModule(conf),
		fxsentry.NewModule(conf),
		// zapOpts contains options to make the logs determistic so we can test the output
		fx.Supply(fx.Annotate(zapOpts, fx.ResultTags(`group:"zap_opts,flatten"`))),
		fx.Invoke(testDPanic),
		fx.Invoke(fx.Annotate(testClient, fx.ParamTags(`optional:"true"`))),
		fx.Invoke(shutdown),
	))

	app.Run()

	// Output:
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Using configuration","conf":{"Mode":"production","Dsn":"","Environment":"prod","Debug":false,"Process":""}}
	// {"level":"dpanic","ts":"2009-11-10T23:00:00.000Z","msg":"Example sentry","error":"test error","extra-data":"some-value"}
	// {"level":"info","ts":"2009-11-10T23:00:00.000Z","msg":"Final configuration","conf":{"Mode":"production","Dsn":"","Environment":"prod","Debug":false,"Process":""}}
}

func testDPanic(logger *zap.Logger) {
	// Panics when log-mode is development, emits a sentry when sentry.dsn != ""
	// The error will be captured as the exception in sentry
	// Any other structured data is captured as "extra"
	// The stacktrace will be of the position where DPanic is called (because errors do not contain stacktrace information)
	logger.DPanic("Example sentry", zap.Error(fmt.Errorf("test error")), zap.String("extra-data", "some-value"))
}

func testClient(client *sentry.Client) {
	// Sentry does not provide a nop-client
	// When no DSN is given in the config, the module does not create a client
	// so we have to test for nil here
	// This is the advanced usage: it is expected that most applications
	// will only need the zap.DPanic integration which is fully transparent
	if client != nil {
		event := sentry.NewEvent()
		event.Message = "My sentry"
		event.Timestamp = time.Now()
		event.Level = sentry.LevelInfo
		client.CaptureEvent(event, nil, nil)
	}
}

func shutdown(sd fx.Shutdowner) {
	sd.Shutdown() //nolint:errcheck
}

var zapOpts = []zap.Option{
	zap.WithCaller(false),
	zap.WithClock(&fixedClock{ts: 1257894000}),
	zap.AddStacktrace(zapcore.PanicLevel), // Disabling stacktraces to keep things reproducible
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
