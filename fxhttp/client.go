package fxhttp

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
)

// ClientMiddleware wraps the transport of http clients, to process the requests they send
// Modules add them to the http_client_middleware value group. Middleware with a higher weight are closer to the network
// The http server has its own middleware, see Middleware
type ClientMiddleware struct {
	RoundTripper func(http.RoundTripper) http.RoundTripper
	Weight       int
}

// NewTransport returns a transport based on http.DefaultTransport with the given tls configuration,
// which is also able to reach servers listening on a unix domain socket, see UnixSocketURL
func NewTransport(tlsConf *tls.Config) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = tlsConf
	registerUnixSocketProtocol(t)
	return t
}

// WrapTransport wraps rt with the given middleware
func WrapTransport(rt http.RoundTripper, middleware []*ClientMiddleware) http.RoundTripper {
	middleware = slices.DeleteFunc(slices.Clone(middleware), func(m *ClientMiddleware) bool { return m == nil })
	slices.SortFunc(middleware, func(a, b *ClientMiddleware) int { return b.Weight - a.Weight })
	for _, m := range middleware {
		rt = m.RoundTripper(rt)
	}
	return rt
}

// maxErrorBodySize bounds how much of an error response body is read
const maxErrorBodySize = 64 << 10

// ErrorFromResponse returns the error response of resp, or nil if resp is not an error (status < 400)
// A body in the standard error shape is parsed as such. Any other body is used as the detail of an Error
// with the status of the response
// It reads the body, but does not close it
func ErrorFromResponse(resp *http.Response) *Error {
	if resp == nil || resp.StatusCode < http.StatusBadRequest {
		return nil
	}

	var body []byte
	if resp.Body != nil {
		body, _ = io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
	}

	e := &Error{}
	if err := json.Unmarshal(body, e); err != nil || e.Status == 0 {
		e = &Error{Detail: strings.TrimSpace(string(body))}
	}
	// The status line is authoritative
	e.Status = resp.StatusCode
	return e.withDefaults()
}
