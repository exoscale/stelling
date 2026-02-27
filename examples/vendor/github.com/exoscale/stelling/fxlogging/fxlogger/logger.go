package fxlogger

import (
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type FxLoggerParams struct {
	fx.In
	Logger *zap.Logger
	Opts   []Option `group:"fxlogger_opts"`
}

// NewFxLogger emits an fxevent.Logger that uses the passed in zap logger
// The fxevent.Logger is used to write out the log messages produces by the fx framework
func NewFxLogger(p FxLoggerParams) fxevent.Logger {
	result := &fxevent.ZapLogger{Logger: p.Logger}
	result.UseLogLevel(zapcore.DebugLevel)
	for _, opt := range p.Opts {
		opt(result)
	}
	return result
}

// Option are constructor parameters that configure the fxevent.Logger
type Option func(l *fxevent.ZapLogger)

// WithLogLevel sets the level at which fx will log the events that happen during system start and stop
func WithLogLevel(lvl zapcore.Level) Option {
	return func(l *fxevent.ZapLogger) {
		l.UseLogLevel(lvl)
	}
}

// WithMinLevel raises the minimum level of the output to the given level
// If the output level is already higher, this is a noop
func WithMinLevel(lvl zapcore.Level) Option {
	return func(l *fxevent.ZapLogger) {
		logger := l.Logger.WithOptions(zap.IncreaseLevel(lvl))
		l.Logger = logger
	}
}

// WithErrorLevel sets the level at which fx will log any error events that happen during system start and stop
func WithErrorLevel(lvl zapcore.Level) Option {
	return func(l *fxevent.ZapLogger) {
		l.UseErrorLevel(lvl)
	}
}
