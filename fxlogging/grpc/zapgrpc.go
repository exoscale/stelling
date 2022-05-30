// Package grpc provides a logger that is compatible with grpclogV2
package grpc

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// See https://github.com/grpc/grpc-go/blob/v1.35.0/grpclog/loggerv2.go#L77-L86
const (
	grpcLvlInfo  = 0
	grpcLvlWarn  = 1
	grpcLvlError = 2
	grpcLvlFatal = 3
)

var (
	// _grpcToZapLevel maps gRPC log levels to zap log levels.
	// See https://pkg.go.dev/go.uber.org/zap@v1.16.0/zapcore#Level
	_grpcToZapLevel = map[int]zapcore.Level{
		grpcLvlInfo:  zapcore.DebugLevel,
		grpcLvlWarn:  zapcore.WarnLevel,
		grpcLvlError: zapcore.ErrorLevel,
		grpcLvlFatal: zapcore.FatalLevel,
	}
)

// NewLogger returns a new Logger.
func NewLogger(l *zap.Logger) *Logger {
	logger := &Logger{
		// Emperically determined the AddCallerSkip value
		// 5 seems to put us in actual grpc code for the majority of logging entries
		// (gRPC has a lot of logging facades)
		// If other gRPC packages than the ones I've checked use more or less facades
		// we won't get good caller information
		delegate:     l.WithOptions(zap.AddCallerSkip(5)).Sugar(),
		levelEnabler: l.Core(),
	}
	return logger
}

// Logger adapts zap's Logger to be compatible with grpclog.LoggerV2
type Logger struct {
	delegate     *zap.SugaredLogger
	levelEnabler zapcore.LevelEnabler
}

// Info implements grpclog.LoggerV2.
func (l *Logger) Info(args ...interface{}) {
	l.delegate.Debug(args...)
}

// Infoln implements grpclog.LoggerV2.
func (l *Logger) Infoln(args ...interface{}) {
	if l.levelEnabler.Enabled(zapcore.DebugLevel) {
		l.delegate.Debug(sprintln(args))
	}
}

// Infof implements grpclog.LoggerV2.
func (l *Logger) Infof(format string, args ...interface{}) {
	l.delegate.Debugf(format, args...)
}

// Warning implements grpclog.LoggerV2.
func (l *Logger) Warning(args ...interface{}) {
	l.delegate.Warn(args...)
}

// Warningln implements grpclog.LoggerV2.
func (l *Logger) Warningln(args ...interface{}) {
	if l.levelEnabler.Enabled(zapcore.WarnLevel) {
		l.delegate.Warn(sprintln(args))
	}
}

// Warningf implements grpclog.LoggerV2.
func (l *Logger) Warningf(format string, args ...interface{}) {
	l.delegate.Warnf(format, args...)
}

// Error implements grpclog.LoggerV2.
func (l *Logger) Error(args ...interface{}) {
	l.delegate.Error(args...)
}

// Errorln implements grpclog.LoggerV2.
func (l *Logger) Errorln(args ...interface{}) {
	if l.levelEnabler.Enabled(zapcore.ErrorLevel) {
		l.delegate.Error(sprintln(args))
	}
}

// Errorf implements grpclog.LoggerV2.
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.delegate.Errorf(format, args...)
}

// Fatal implements grpclog.LoggerV2.
func (l *Logger) Fatal(args ...interface{}) {
	l.delegate.Fatal(args...)
}

// Fatalln implements grpclog.LoggerV2.
func (l *Logger) Fatalln(args ...interface{}) {
	if l.levelEnabler.Enabled(zapcore.FatalLevel) {
		l.delegate.Fatal(sprintln(args))
	}
}

// Fatalf implements grpclog.LoggerV2.
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.delegate.Fatalf(format, args...)
}

// V implements grpclog.LoggerV2.
func (l *Logger) V(level int) bool {
	return l.levelEnabler.Enabled(_grpcToZapLevel[level])
}

func sprintln(args []interface{}) string {
	s := fmt.Sprintln(args...)
	// Drop the new line character added by Sprintln
	return s[:len(s)-1]
}
