package http

import (
	"context"
	"net/http"

	"github.com/exoscale/stelling/fxlogging/interceptor"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type requestIdContextKey struct{}

var requestIdCtxKey = &requestIdContextKey{}

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

		requestId := uuid.New()

		ctx := context.WithValue(r.Context(), requestIdCtxKey, requestId)

		ww.Header().Add("X-Request-Id", requestId.String())

		fields := []zapcore.Field{
			zap.String("Request-Id", requestId.String()),
			zap.String("Request-Method", r.Method),
			zap.String("Request-URI", r.RequestURI),
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
