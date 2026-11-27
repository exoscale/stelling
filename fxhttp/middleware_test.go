package fxhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func newTestRequest(t *testing.T) *http.Request {
	t.Helper()
	return httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
}

func TestRequestIDHandler(t *testing.T) {
	serve := func(t *testing.T, r *http.Request) (string, string) {
		t.Helper()
		var fromContext, fromHeader string
		requestIDHandler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			fromContext = RequestIDFromContext(r.Context())
			fromHeader = r.Header.Get(RequestIDHeader)
		})).ServeHTTP(httptest.NewRecorder(), r)
		return fromContext, fromHeader
	}

	t.Run("Should keep the incoming request id", func(t *testing.T) {
		r := newTestRequest(t)
		r.Header.Set(RequestIDHeader, "req-123")
		fromContext, fromHeader := serve(t, r)
		require.Equal(t, "req-123", fromContext)
		require.Equal(t, "req-123", fromHeader)
	})

	t.Run("Should generate a request id when absent", func(t *testing.T) {
		fromContext, fromHeader := serve(t, newTestRequest(t))
		require.NotEmpty(t, fromContext)
		require.Equal(t, fromContext, fromHeader)
	})

	t.Run("Should replace invalid request ids", func(t *testing.T) {
		for _, invalid := range []string{strings.Repeat("a", maxRequestIDLength+1), "req\x00id", "req id", "réq"} {
			r := newTestRequest(t)
			r.Header.Set(RequestIDHeader, invalid)
			fromContext, fromHeader := serve(t, r)
			require.NotEqual(t, invalid, fromContext)
			require.Equal(t, fromContext, fromHeader)
		}
	})

	t.Run("Should return the empty string without the middleware", func(t *testing.T) {
		require.Equal(t, "", RequestIDFromContext(context.Background()))
	})
}

func TestDrainHandler(t *testing.T) {
	serve := func(t *testing.T, drain *DrainState) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		DrainHandler(drain)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, newTestRequest(t))
		return rec
	}

	t.Run("Should not close connections while running", func(t *testing.T) {
		require.Equal(t, "", serve(t, NewDrainState()).Header().Get("Connection"))
	})

	t.Run("Should close connections once draining", func(t *testing.T) {
		drain := NewDrainState()
		require.True(t, drain.EnterDraining())
		require.False(t, drain.EnterDraining(), "entering draining twice should report no change")
		require.Equal(t, "close", serve(t, drain).Header().Get("Connection"))
	})

	t.Run("Should handle a nil drain state", func(t *testing.T) {
		require.Equal(t, "", serve(t, nil).Header().Get("Connection"))
	})
}

func TestIAMIdentityHandler(t *testing.T) {
	serve := func(t *testing.T, r *http.Request) string {
		t.Helper()
		var identityID string
		IAMIdentityHandler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			identityID = IAMIdentityIDFromContext(r.Context())
		})).ServeHTTP(httptest.NewRecorder(), r)
		return identityID
	}

	t.Run("Should store the identity id", func(t *testing.T) {
		r := newTestRequest(t)
		r.Header.Set(IAMIdentityIDHeader, "identity-1")
		require.Equal(t, "identity-1", serve(t, r))
	})

	t.Run("Should return the empty string without the header", func(t *testing.T) {
		require.Equal(t, "", serve(t, newTestRequest(t)))
	})
}

func TestErrorRecorder(t *testing.T) {
	t.Run("Should record the error written by WriteError", func(t *testing.T) {
		r := newTestRequest(t)
		r = r.WithContext(ContextWithErrorRecorder(r.Context()))
		WriteError(httptest.NewRecorder(), r, &Error{Status: http.StatusNotFound, Detail: "no thing"})

		require.Equal(t, &Error{Status: http.StatusNotFound, Type: "#/problems/not-found", Title: "The resource is not found.", Detail: "no thing"}, RecordedError(r.Context()))
	})

	t.Run("Should keep an existing recorder", func(t *testing.T) {
		ctx := ContextWithErrorRecorder(context.Background())
		recordError(ctx, &Error{Status: http.StatusConflict})
		require.Equal(t, http.StatusConflict, RecordedError(ContextWithErrorRecorder(ctx)).Status)
	})

	t.Run("Should be a no-op without a recorder", func(t *testing.T) {
		recordError(context.Background(), &Error{})
		require.Nil(t, RecordedError(context.Background()))
	})
}
