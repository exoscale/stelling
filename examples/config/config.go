package config

import (
	"time"

	"github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/fxgrpc"
	"github.com/exoscale/stelling/fxhttp"
	"github.com/exoscale/stelling/fxlogging"
	"github.com/exoscale/stelling/fxmetrics"
	"github.com/exoscale/stelling/fxpprof"
	"github.com/exoscale/stelling/fxsentry"
	"github.com/exoscale/stelling/fxtracing"
)

// Config values can be overridden in an environment variable using the CONFIG_ prefix.
// For example:
// export CONFIG_GREETING_MESSAGE="some new greeting"

// Define config sub-categories with separate structs like this. For example, you could group all
// parameters related to connecting to Prometheus in a Prometheus struct.
type Greeting struct {
	Message string `default:"this is the default greeting!"`
}

type Config struct {
	fxgrpc.Server
	fxlogging.Logging
	fxpprof.Pprof
	fxmetrics.Metrics
	fxtracing.Tracing
	fxsentry.Sentry
	HttpServer fxhttp.Server

	Greeting

	FeatureFlag    bool
	Mode           string        `default:"high" validate:"oneof=low medium high"`
	RequiredNumber int           `validate:"required"`
	Interval       time.Duration `default:"1m"`
	APIKey         config.Secret `default:"my-key"`
}
