package interceptor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"
)

func withTestRequestLogger(t *testing.T, cb func(handler http.Handler, logs *observer.ObservedLogs), opts ...HTTPOption) {
	t.Helper()

	core, logs := observer.New(zapcore.DebugLevel)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.WrapCore(func(_ zapcore.Core) zapcore.Core { return core })))

	handler := NewRequestLogger(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		LoggerFromContext(r.Context()).Info("handler log")
		w.WriteHeader(http.StatusAccepted)
	}), opts...)

	cb(handler, logs)
}

func TestRequestLogger(t *testing.T) {
	t.Run("Should enrich logger with request id header", func(t *testing.T) {
		rid := uuid.NewString()
		run := func(handler http.Handler, logs *observer.ObservedLogs) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
			req.Header.Set("x-request-id", rid)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			require.NotEmpty(t, rr.Header().Get("X-Trace-Id"))

			handlerLog := requireLogMessage(t, logs.AllUntimed(), "handler log")
			require.Contains(t, handlerLog.ContextMap(), "request_id")
			require.Equal(t, rid, handlerLog.ContextMap()["request_id"])

			requestLog := requireLogMessage(t, logs.AllUntimed(), "Handled request")
			require.Contains(t, requestLog.ContextMap(), "request_id")
			require.Equal(t, rid, requestLog.ContextMap()["request_id"])
		}
		withTestRequestLogger(t, run)
	})

	t.Run("Should enrich logger with headerFields", func(t *testing.T) {
		rid := uuid.NewString()
		run := func(handler http.Handler, logs *observer.ObservedLogs) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
			req.Header.Set("x-correlation-id", rid)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			handlerLog := requireLogMessage(t, logs.AllUntimed(), "handler log")
			require.Contains(t, handlerLog.ContextMap(), "correlation_id")
			require.Equal(t, rid, handlerLog.ContextMap()["correlation_id"])

			requestLog := requireLogMessage(t, logs.AllUntimed(), "Handled request")
			require.Contains(t, requestLog.ContextMap(), "correlation_id")
			require.Equal(t, rid, requestLog.ContextMap()["correlation_id"])
		}
		headerFields := map[string]string{
			"x-correlation-id": "correlation_id",
		}
		withTestRequestLogger(t, run, WithHTTPHeaderFields(headerFields))
	})

	t.Run("Should not enrich logger when request id header is missing", func(t *testing.T) {
		run := func(handler http.Handler, logs *observer.ObservedLogs) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			handlerLog := requireLogMessage(t, logs.AllUntimed(), "handler log")
			require.NotContains(t, handlerLog.ContextMap(), "request_id")

			requestLog := requireLogMessage(t, logs.AllUntimed(), "Handled request")
			require.NotContains(t, requestLog.ContextMap(), "request_id")
		}
		withTestRequestLogger(t, run)
	})
}

func requireLogMessage(t *testing.T, entries []observer.LoggedEntry, message string) observer.LoggedEntry {
	t.Helper()
	for _, entry := range entries {
		if entry.Message == message {
			return entry
		}
	}
	require.Failf(t, "missing log message", "message %q was not logged", message)
	return observer.LoggedEntry{}
}
