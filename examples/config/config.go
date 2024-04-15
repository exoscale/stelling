package config

import (
	"time"

	"github.com/exoscale/stelling/fxgrpc"
	"github.com/exoscale/stelling/fxlogging"
	"github.com/exoscale/stelling/fxmetrics"
	"github.com/exoscale/stelling/fxpprof"
	"github.com/exoscale/stelling/fxsentry"
	"github.com/exoscale/stelling/fxtracing"
)

type Config struct {
	fxgrpc.Server
	fxlogging.Logging
	fxpprof.Pprof
	fxmetrics.Metrics
	fxtracing.Tracing
	fxsentry.Sentry

	FeatureFlag    bool
	Mode           string        `default:"high" validate:"oneof=low medium high"`
	RequiredNumber int           `validate:"required"`
	Interval       time.Duration `default:"1m"`
}
