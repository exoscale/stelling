package fxtracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/exoscale/stelling/fxhttp"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func newRecordingTracerProvider(t *testing.T) (*sdktrace.TracerProvider, *tracetest.SpanRecorder) {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return tp, sr
}

func TestHttpMiddlewareRecordedError(t *testing.T) {
	serve := func(t *testing.T, e *fxhttp.Error) sdktrace.ReadOnlySpan {
		t.Helper()
		tp, sr := newRecordingTracerProvider(t)
		h := NewHttpMiddleware(tp).Middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fxhttp.WriteError(w, r, e)
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
		spans := sr.Ended()
		require.Len(t, spans, 1)
		return spans[0]
	}

	t.Run("Should record server errors on the span", func(t *testing.T) {
		span := serve(t, &fxhttp.Error{Status: http.StatusInternalServerError, Type: "#/store/unavailable"})
		require.Equal(t, codes.Error, span.Status().Code)
		var errorType string
		for _, a := range span.Attributes() {
			if a.Key == "error.type" {
				errorType = a.Value.AsString()
			}
		}
		require.Equal(t, "#/store/unavailable", errorType)
		require.NotEmpty(t, span.Events(), "the error should be recorded as an event")
	})

	t.Run("Should not mark client errors as span errors", func(t *testing.T) {
		span := serve(t, &fxhttp.Error{Status: http.StatusBadRequest})
		require.Equal(t, codes.Unset, span.Status().Code)
		require.Empty(t, span.Events())
	})
}

func TestHttpClientMiddleware(t *testing.T) {
	tp, sr := newRecordingTracerProvider(t)

	var traceparent string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		traceparent = r.Header.Get("traceparent")
	}))
	t.Cleanup(srv.Close)

	client := &http.Client{Transport: fxhttp.WrapTransport(http.DefaultTransport, []*fxhttp.ClientMiddleware{NewHttpClientMiddleware(tp).Middleware})}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	require.NotEmpty(t, traceparent, "the trace context should be propagated")
	require.Len(t, sr.Ended(), 1)
}
