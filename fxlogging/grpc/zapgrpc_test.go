package grpc

import (
	"fmt"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zapgrpc"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc/grpclog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggerDebugExpected(t *testing.T) {
	checkMessages(t, zapcore.DebugLevel, zapcore.DebugLevel, []string{
		"hello",
		"s1s21 2 3s34s56",
		"hello world",
		"",
		"foo",
		"foo bar",
		"s1 s2 1 2 3 s3 4 s5 6",
	}, func(logger *Logger) {
		logger.Info("hello")
		logger.Info("s1", "s2", 1, 2, 3, "s3", 4, "s5", 6)
		logger.Infof("%s world", "hello")
		logger.Infoln()
		logger.Infoln("foo")
		logger.Infoln("foo", "bar")
		logger.Infoln("s1", "s2", 1, 2, 3, "s3", 4, "s5", 6)
	})
}

func TestLoggerWarningExpected(t *testing.T) {
	checkMessages(t, zapcore.DebugLevel, zapcore.WarnLevel, []string{
		"hello",
		"s1s21 2 3s34s56",
		"hello world",
		"",
		"foo",
		"foo bar",
		"s1 s2 1 2 3 s3 4 s5 6",
	}, func(logger *Logger) {
		logger.Warning("hello")
		logger.Warning("s1", "s2", 1, 2, 3, "s3", 4, "s5", 6)
		logger.Warningf("%s world", "hello")
		logger.Warningln()
		logger.Warningln("foo")
		logger.Warningln("foo", "bar")
		logger.Warningln("s1", "s2", 1, 2, 3, "s3", 4, "s5", 6)
	})
}

func TestLoggerErrorExpected(t *testing.T) {
	checkMessages(t, zapcore.DebugLevel, zapcore.ErrorLevel, []string{
		"hello",
		"s1s21 2 3s34s56",
		"hello world",
		"",
		"foo",
		"foo bar",
		"s1 s2 1 2 3 s3 4 s5 6",
	}, func(logger *Logger) {
		logger.Error("hello")
		logger.Error("s1", "s2", 1, 2, 3, "s3", 4, "s5", 6)
		logger.Errorf("%s world", "hello")
		logger.Errorln()
		logger.Errorln("foo")
		logger.Errorln("foo", "bar")
		logger.Errorln("s1", "s2", 1, 2, 3, "s3", 4, "s5", 6)
	})
}

func TestLoggerV(t *testing.T) {
	tests := []struct {
		zapLevel     zapcore.Level
		grpcEnabled  []int
		grpcDisabled []int
	}{
		{
			zapLevel:     zapcore.DebugLevel,
			grpcEnabled:  []int{grpcLvlInfo, grpcLvlWarn, grpcLvlError, grpcLvlFatal},
			grpcDisabled: []int{}, // everything is enabled, nothing is disabled
		},
		{
			zapLevel:     zapcore.InfoLevel,
			grpcEnabled:  []int{grpcLvlWarn, grpcLvlError, grpcLvlFatal},
			grpcDisabled: []int{grpcLvlInfo},
		},
		{
			zapLevel:     zapcore.WarnLevel,
			grpcEnabled:  []int{grpcLvlWarn, grpcLvlError, grpcLvlFatal},
			grpcDisabled: []int{grpcLvlInfo},
		},
		{
			zapLevel:     zapcore.ErrorLevel,
			grpcEnabled:  []int{grpcLvlError, grpcLvlFatal},
			grpcDisabled: []int{grpcLvlInfo, grpcLvlWarn},
		},
		{
			zapLevel:     zapcore.DPanicLevel,
			grpcEnabled:  []int{grpcLvlFatal},
			grpcDisabled: []int{grpcLvlInfo, grpcLvlWarn, grpcLvlError},
		},
		{
			zapLevel:     zapcore.PanicLevel,
			grpcEnabled:  []int{grpcLvlFatal},
			grpcDisabled: []int{grpcLvlInfo, grpcLvlWarn, grpcLvlError},
		},
		{
			zapLevel:     zapcore.FatalLevel,
			grpcEnabled:  []int{grpcLvlFatal},
			grpcDisabled: []int{grpcLvlInfo, grpcLvlWarn, grpcLvlError},
		},
	}
	for _, tst := range tests {
		for _, grpcLvl := range tst.grpcEnabled {
			t.Run(fmt.Sprintf("enabled %s %d", tst.zapLevel, grpcLvl), func(t *testing.T) {
				checkLevel(t, tst.zapLevel, true, func(logger *Logger) bool {
					return logger.V(grpcLvl)
				})
			})
		}
		for _, grpcLvl := range tst.grpcDisabled {
			t.Run(fmt.Sprintf("disabled %s %d", tst.zapLevel, grpcLvl), func(t *testing.T) {
				checkLevel(t, tst.zapLevel, false, func(logger *Logger) bool {
					return logger.V(grpcLvl)
				})
			})
		}
	}
}

// We already depend on grpc in stelling, so we can just put this test here
// If we ever make the packages individual modules to reduce the amount of dependencies
// clients need to pull in, we may want to move this test to the fxgrpc package
func TestLoggerV2(t *testing.T) {
	core, observedLogs := observer.New(zapcore.InfoLevel)
	zlog := zap.New(core)

	grpclog.SetLoggerV2(zapgrpc.NewLogger(zlog))

	grpclog.Info("hello from grpc")

	logs := observedLogs.TakeAll()
	require.Len(t, logs, 1, "Expected one log entry.")
	entry := logs[0]

	assert.Equal(t, zapcore.InfoLevel, entry.Level,
		"Log entry level did not match.")
	assert.Equal(t, "hello from grpc", entry.Message,
		"Log entry message did not match.")
}

func checkLevel(
	tb testing.TB,
	enab zapcore.LevelEnabler,
	expectedBool bool,
	f func(*Logger) bool,
) {
	tb.Helper()
	withLogger(enab, func(logger *Logger, observedLogs *observer.ObservedLogs) {
		actualBool := f(logger)
		if expectedBool {
			require.True(tb, actualBool)
		} else {
			require.False(tb, actualBool)
		}
	})
}

func checkMessages(
	tb testing.TB,
	enab zapcore.LevelEnabler,
	expectedLevel zapcore.Level,
	expectedMessages []string,
	f func(*Logger),
) {
	tb.Helper()
	if expectedLevel == zapcore.FatalLevel {
		expectedLevel = zapcore.WarnLevel
	}
	withLogger(enab, func(logger *Logger, observedLogs *observer.ObservedLogs) {
		f(logger)
		logEntries := observedLogs.All()
		require.Equal(tb, len(expectedMessages), len(logEntries))
		for i, logEntry := range logEntries {
			require.Equal(tb, expectedLevel, logEntry.Level)
			require.Equal(tb, expectedMessages[i], logEntry.Message)
		}
	})
}

func withLogger(
	enab zapcore.LevelEnabler,
	f func(*Logger, *observer.ObservedLogs),
) {
	core, observedLogs := observer.New(enab)
	f(NewLogger(zap.New(core)), observedLogs)
}
