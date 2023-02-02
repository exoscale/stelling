package fxsentry

import (
	"context"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/TheZeroSlave/zapsentry"
	sentry "github.com/getsentry/sentry-go"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewModule(conf SentryConfig) fx.Option {
	if conf.SentryConfig().Dsn == "" {
		return fx.Options()
	}
	return fx.Options(
		fx.Supply(fx.Annotate(conf, fx.As(new(SentryConfig)))),
		fx.Provide(ProvideSentryClient),
		fx.Decorate(ProvideSentryLogger),
	)
}

type SentryConfig interface {
	SentryConfig() *Sentry
}

type Sentry struct {
	// Dsn contains the sentry Dsn
	// Sentry integration is disabled if this is empty
	Dsn string
	// Environment is reported as the 'environment' tag in any sentry events
	Environment string `default:"prod" validate:"oneof=dev lab preprod prod"`
	// Debug controls whether sentry emits debugs logs about its own actions
	Debug bool
	// Process is the name of the current process, will be reported in the 'process' tag
	// The lib will try to deduce a value from the runtime if not set
	Process string
}

func (s *Sentry) SentryConfig() *Sentry {
	return s
}

func (s *Sentry) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if s == nil {
		return nil
	}

	enc.AddString("dsn", s.Dsn)
	enc.AddString("environment", s.Environment)
	enc.AddBool("debug", s.Debug)
	enc.AddString("process", s.Process)

	return nil
}

func NewSentryClient(conf SentryConfig) (*sentry.Client, error) {
	sentryConf := conf.SentryConfig()

	if sentryConf.Dsn == "" {
		return nil, nil
	}

	hostname := ""
	hostname, err := os.Hostname()
	if err != nil {
		hostname = ""
	}
	version := "undefined"
	// We're not using info.Main.Version because it always shows `(devel)` for the main
	// module, unless installed through go install
	// Hopefully the resolution to this issue improves things: https://github.com/golang/go/issues/50603
	if info, ok := debug.ReadBuildInfo(); ok {
		// I think a common lisper snuck code into go: why use a map when you have lists!
		for _, item := range info.Settings {
			if item.Key == "vcs.revision" {
				version = item.Value
				break
			}
		}
	}

	opts := sentry.ClientOptions{
		Dsn:              sentryConf.Dsn,
		ServerName:       hostname,
		Environment:      sentryConf.Environment,
		Release:          version,
		Debug:            sentryConf.Debug,
		AttachStacktrace: true,
	}

	// Mutate the top level scope with some extra useful information
	sentry.ConfigureScope(func(scope *sentry.Scope) {
		procName := sentryConf.Process
		if procName == "" {
			pathName, err := os.Executable()
			// We'll ignore errors
			if err == nil && pathName != "" {
				procName = filepath.Base(pathName)
			}
		}
		if procName != "" {
			scope.SetTag("process", filepath.Base(procName))
		}
	})

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
	cfg := zapsentry.Configuration{
		Level:             zapcore.DPanicLevel,
		EnableBreadcrumbs: false,
	}

	// Returns a noopcore if we error, so we can still safely attach to the logger
	core, _ := zapsentry.NewCore(cfg, zapsentry.NewSentryClientFromClient(client))

	return zapsentry.AttachCoreToLogger(core, logger)
}
