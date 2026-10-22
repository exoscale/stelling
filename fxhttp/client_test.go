package fxhttp

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnixSocketTransport(t *testing.T) {
	// Socket paths are limited in length: avoid the long default test temp dir
	dir, err := os.MkdirTemp("", "fxhttp")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "server.sock")

	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", socket)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.URL.Path+"?"+r.URL.RawQuery)
	})}
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() { _ = srv.Close() })

	client := &http.Client{Transport: NewTransport(nil)}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, UnixSocketURL(socket)+"/things/42?expand=true", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "/things/42?expand=true", string(body))

	t.Run("Should refuse redirects to a socket", func(t *testing.T) {
		remote := httptest.NewServer(http.RedirectHandler(UnixSocketURL(socket)+"/things/42", http.StatusFound))
		t.Cleanup(remote.Close)

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, remote.URL, nil)
		require.NoError(t, err)
		resp, err := client.Do(req) //nolint:bodyclose // The request fails
		require.ErrorContains(t, err, "redirects to a unix domain socket are not allowed")
		require.Nil(t, resp)
	})
}

func TestWrapTransport(t *testing.T) {
	var calls []string
	middleware := func(name string, weight int) *ClientMiddleware {
		return &ClientMiddleware{
			RoundTripper: func(next http.RoundTripper) http.RoundTripper {
				return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
					calls = append(calls, name)
					return next.RoundTrip(r)
				})
			},
			Weight: weight,
		}
	}
	base := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls = append(calls, "network")
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})

	rt := WrapTransport(base, []*ClientMiddleware{middleware("inner", 50), nil, middleware("outer", 10)})
	resp, err := rt.RoundTrip(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil))
	require.NoError(t, err)
	_ = resp.Body.Close()

	require.Equal(t, []string{"outer", "inner", "network"}, calls)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestErrorFromResponse(t *testing.T) {
	response := func(status int, body string) *http.Response {
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
	}

	t.Run("Should return nil for successful responses", func(t *testing.T) {
		resp := response(http.StatusOK, "{}")
		defer func() { _ = resp.Body.Close() }()
		require.Nil(t, ErrorFromResponse(resp))
		require.Nil(t, ErrorFromResponse(nil))
	})

	t.Run("Should parse the standard error shape", func(t *testing.T) {
		resp := response(http.StatusConflict, `{"type":"#/thing/exists","title":"Thing exists","status":409,"detail":"thing 42","thing-id":42}`)
		defer func() { _ = resp.Body.Close() }()
		e := ErrorFromResponse(resp)
		require.Equal(t, http.StatusConflict, e.Status)
		require.Equal(t, "#/thing/exists", e.Type)
		require.Equal(t, "Thing exists", e.Title)
		require.Equal(t, "thing 42", e.Detail)
		require.Contains(t, e.Extensions, "thing-id")
	})

	t.Run("Should use the body as detail when it is not an error response", func(t *testing.T) {
		resp := response(http.StatusBadGateway, "upstream down\n")
		defer func() { _ = resp.Body.Close() }()
		e := ErrorFromResponse(resp)
		require.Equal(t, &Error{Status: http.StatusBadGateway, Type: typeBlank, Title: "Bad Gateway", Detail: "upstream down"}, e)
	})

	t.Run("Should prefer the status of the response", func(t *testing.T) {
		resp := response(http.StatusInternalServerError, `{"type":"x","title":"y","status":400,"detail":"z"}`)
		defer func() { _ = resp.Body.Close() }()
		e := ErrorFromResponse(resp)
		require.Equal(t, http.StatusInternalServerError, e.Status)
	})
}
