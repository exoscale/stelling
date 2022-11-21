package fxsentry

import (
	"context"
	"os"
	"runtime/debug"
	"time"

	"github.com/TheZeroSlave/zapsentry"
	sentry "github.com/getsentry/sentry-go"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Module = fx.Options(
	fx.Provide(ProvideSentryClient),
	fx.Decorate(fx.Annotate(ProvideSentryLogger, fx.ParamTags(``, `optional:"true"`))),
)

type SentryConfig interface {
	GetSentry() *Sentry
}

type Sentry struct {
	// Dsn contains the sentry Dsn
	// Sentry integration is disabled if this is empty
	Dsn         string
	Environment string `default:"prod" validate:"oneof=dev lab preprod prod"`
	Debug       bool
}

func (s *Sentry) GetSentry() *Sentry {
	return s
}

func (s *Sentry) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if s == nil {
		return nil
	}

	enc.AddString("dsn", s.Dsn)
	enc.AddString("environment", s.Environment)
	enc.AddBool("debug", s.Debug)

	return nil
}

func NewSentryClient(conf SentryConfig) (*sentry.Client, error) {
	sentryConf := conf.GetSentry()

	if sentryConf.Dsn == "" {
		return nil, nil
	}

	hostname := ""
	hostname, err := os.Hostname()
	if err != nil {
		hostname = ""
	}
	version := "undefined"
	if info, ok := debug.ReadBuildInfo(); ok {
		version = info.Main.Version
	}

	opts := sentry.ClientOptions{
		Dsn:         sentryConf.Dsn,
		ServerName:  hostname,
		Environment: sentryConf.Environment,
		Release:     version,
		Debug:       sentryConf.Debug,
	}

	return sentry.NewClient(opts)
}

func ProvideSentryClient(lc fx.Lifecycle, conf SentryConfig) (*sentry.Client, error) {
	client, err := NewSentryClient(conf)
	if err != nil {
		return nil, err
	}

	if client != nil {
		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				client.Flush(2 * time.Second)
				return nil
			},
		})
	}
	return client, nil
}

func ProvideSentryLogger(logger *zap.Logger, client *sentry.Client) *zap.Logger {
	if client == nil {
		return logger
	}
	cfg := zapsentry.Configuration{
		Level:             zapcore.DPanicLevel,
		EnableBreadcrumbs: false,
	}

	// Returns a noopcore if we error, so we can still safely attach to the logger
	core, _ := zapsentry.NewCore(cfg, zapsentry.NewSentryClientFromClient(client))

	// To use breadcrumbs feature we need to create a new scope explicitly
	logger = logger.With(zapsentry.NewScope())

	return zapsentry.AttachCoreToLogger(core, logger)
}
