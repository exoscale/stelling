package fxhttp

import (
	"context"
	"encoding/base32"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// unixSocketScheme is the url scheme of http requests sent over a unix domain socket, see UnixSocketURL
const unixSocketScheme = "http+unix"

// unixSocketHostSuffix is reserved to mark hosts encoding a socket path
const unixSocketHostSuffix = ".sock"

var unixSocketHostEncoding = base32.HexEncoding.WithPadding(base32.NoPadding)

// UnixSocketURL returns the base url to reach the http server listening on socketPath
// The path is encoded in the host, so the url can be extended with paths and queries as usual
func UnixSocketURL(socketPath string) string {
	return unixSocketScheme + "://" + strings.ToLower(unixSocketHostEncoding.EncodeToString([]byte(socketPath))) + unixSocketHostSuffix
}

func decodeUnixSocketHost(host string) (string, error) {
	if !strings.HasSuffix(host, unixSocketHostSuffix) {
		return "", fmt.Errorf("invalid socket host %q", host)
	}

	encoded := strings.TrimSuffix(host, unixSocketHostSuffix)
	decoded, err := unixSocketHostEncoding.DecodeString(strings.ToUpper(encoded))
	if err != nil {
		return "", fmt.Errorf("decode socket host %q: %w", encoded, err)
	}

	return string(decoded), nil
}

// registerUnixSocketProtocol makes t able to send requests to UnixSocketURL urls
func registerUnixSocketProtocol(t *http.Transport) {
	unixTransport := t.Clone()
	unixTransport.Proxy = nil
	unixTransport.DialContext = func(ctx context.Context, _, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			// addr may not include a port
			host = strings.TrimSuffix(addr, ":80")
		}

		socketPath, err := decodeUnixSocketHost(host)
		if err != nil {
			return nil, err
		}

		var d net.Dialer
		return d.DialContext(ctx, "unix", socketPath)
	}

	t.RegisterProtocol(unixSocketScheme, unixSocketRoundTripper{t: unixTransport})
}

type unixSocketRoundTripper struct {
	t *http.Transport
}

// RoundTrip sends req as a plain http request: the dialer of the transport connects to the socket encoded in its host
// Redirects to a socket are refused, unless they come from that same socket: a remote server must not make the
// client reach the local sockets of its host
func (rt unixSocketRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if prev := req.Response; prev != nil &&
		(prev.Request.URL.Scheme != unixSocketScheme || prev.Request.URL.Host != req.URL.Host) {
		return nil, errors.New("redirects to a unix domain socket are not allowed")
	}
	rewrite := req.Clone(req.Context())
	rewrite.URL.Scheme = "http"
	resp, err := rt.t.RoundTrip(rewrite)
	if resp != nil {
		// The response belongs to the request the client sent, not to its rewrite, so redirects see the socket url
		resp.Request = req
	}
	return resp, err
}
