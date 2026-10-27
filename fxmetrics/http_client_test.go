package fxmetrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/exoscale/stelling/fxhttp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestHttpClientMiddleware(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(srv.Close)

	reg := prometheus.NewRegistry()
	result, err := NewHttpClientMiddleware(reg)
	require.NoError(t, err)

	client := &http.Client{Transport: fxhttp.WrapTransport(http.DefaultTransport, []*fxhttp.ClientMiddleware{result.Middleware})}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	families, err := reg.Gather()
	require.NoError(t, err)
	found := false
	for _, f := range families {
		if f.GetName() == "http_client_requests_total" {
			found = true
			require.Len(t, f.GetMetric(), 1)
			require.InDelta(t, 1, f.GetMetric()[0].GetCounter().GetValue(), 0)
			labels := map[string]string{}
			for _, l := range f.GetMetric()[0].GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			require.Equal(t, map[string]string{"code": "418", "method": "get"}, labels)
		}
	}
	require.True(t, found, "http_client_requests_total should be registered")
}
