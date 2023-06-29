package main

import (
	"time"

	sconfig "github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/fxlogging"
	"github.com/exoscale/stelling/fxsentry"
	"github.com/getsentry/sentry-go"
	"go.uber.org/fx"
)

type Config struct {
	fxlogging.Logging
	fxsentry.Sentry
}

func main() {

	conf := &Config{}
	args := []string{"sentry-test", "--logging.mode", "production"}
	if err := sconfig.Load(conf, args); err != nil {
		panic(err)
	}
	app := fx.New(fx.Options(
		fxlogging.NewModule(conf),
		fxsentry.NewModule(conf),
		fx.Invoke(fx.Annotate(testClient, fx.ParamTags(`optional:"true"`))),
		fx.Invoke(shutdown),
	))

	app.Run()
}

func testClient(client *sentry.Client) {
	// This is the advanced usage: it is expected that most applications
	// will only need the zap.DPanic integration which is fully transparent
	event := sentry.NewEvent()
	event.Message = "My sentry"
	event.Timestamp = time.Now()
	event.Level = sentry.LevelInfo
	client.CaptureEvent(event, nil, nil)
}

func shutdown(sd fx.Shutdowner) {
	sd.Shutdown() //nolint:errcheck
}
