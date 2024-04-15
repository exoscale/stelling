package config

import (
	"time"

	"github.com/exoscale/stelling/fxgrpc"
	"github.com/exoscale/stelling/fxlogging"
	"github.com/exoscale/stelling/fxmetrics"
	"github.com/exoscale/stelling/fxpprof"
	"github.com/exoscale/stelling/fxsentry"
	"github.com/exoscale/stelling/fxtracing"
	"go.uber.org/zap/zapcore"
)

type Config struct {
	fxgrpc.Server
	fxlogging.Logging
	fxpprof.Pprof
	fxmetrics.OtlpMetrics
	fxtracing.Tracing
	fxsentry.Sentry

	FeatureFlag    bool
	Mode           string        `default:"high" validate:"oneof=low medium high"`
	RequiredNumber int           `validate:"required"`
	Interval       time.Duration `default:"1m"`
}

func (c *Config) MarshalLogObject(enc zapcore.ObjectEncoder) error {

	if err := enc.AddReflected("server", c.Server); err != nil {
		return err
	}
	if err := enc.AddReflected("logging", c.Logging); err != nil {
		return err
	}
	if err := enc.AddReflected("pprof", c.Pprof); err != nil {
		return err
	}

	if err := enc.AddReflected("otlpmetrics", c.OtlpMetrics); err != nil {
		return err
	}
	if err := enc.AddReflected("tracing", c.Tracing); err != nil {
		return err
	}
	if err := enc.AddReflected("sentry", c.Sentry); err != nil {
		return err
	}

	enc.AddBool("featureflag", c.FeatureFlag)
	enc.AddString("mode", c.Mode)
	enc.AddInt("required-number", c.RequiredNumber)
	enc.AddDuration("interval", c.Interval)

	return nil
}
