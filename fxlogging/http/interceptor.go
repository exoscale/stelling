package http

import (
	"net/http"

	"github.com/exoscale/stelling/fxlogging/interceptor"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type WrapResponseWriter struct {
	http.ResponseWriter

	StatusCode int
}

var _ http.ResponseWriter = (*WrapResponseWriter)(nil)

func NewWrapResponseWriter(w http.ResponseWriter) *WrapResponseWriter {
	return &WrapResponseWriter{ResponseWriter: w}
}

func (w *WrapResponseWriter) WriteHeader(statusCode int) {
	w.StatusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func NewRequestLogger(logger *zap.Logger, wrapped http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := NewWrapResponseWriter(w)

		ctx := r.Context()

		traceid, ok := interceptor.TraceIdFromContext(ctx)
		if !ok {
			ctx = interceptor.ContextWithTraceId(ctx, traceid)
		}

		ww.Header().Add("X-Trace-Id", traceid)

		fields := []zapcore.Field{
			zap.String("http.method", r.Method),
			zap.String("http.uri", r.RequestURI),
			zap.String("otlp.trace_id", traceid),
		}

		if rUser, ok := r.Header["X-Forwarded-User"]; ok {
			if len(rUser) > 0 {
				fields = append(fields, zap.String("X-Forwarded-User", rUser[0]))
			}
		}

		l := logger.With(fields...)
		ctx = interceptor.ContextWithLogger(ctx, l)
		r = r.WithContext(ctx)

		wrapped.ServeHTTP(ww, r)

		l.Info(
			"Handled request",
			zap.Int("status", ww.StatusCode),
		)
	})
}
