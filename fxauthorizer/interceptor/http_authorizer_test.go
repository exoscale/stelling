package interceptor

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/stretchr/testify/require"
)

func TestCompileHttpProgram(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		isError bool
	}{
		{
			name:    "Should return an error if the program can't be parsed",
			input:   "foobar",
			isError: true,
		},
		{
			name:    "Should return an error if the program references a variable that isn't defined",
			input:   "req.notthere == 'true'",
			isError: true,
		},
		{
			name:    "Should not return an error for a valid program",
			input:   "request.method == 'GET'",
			isError: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := compileHttpCelProgram(tc.input)
			if tc.isError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, output)
			}
		})
	}
}

func TestNewHttpAuthorizer(t *testing.T) {
	t.Run("Should return an error when the CEL rule is invalid", func(t *testing.T) {
		output, err := NewHttpAuthorizer("foobar")
		require.Nil(t, output)
		require.Error(t, err)
	})

	t.Run("Should return an authorizer with correct defaults", func(t *testing.T) {
		output, err := NewHttpAuthorizer("true")
		require.NoError(t, err)

		require.Equal(t, TokenFormatNone, output.authTokenFormat)
		require.NotNil(t, output.rule)
		require.Nil(t, output.tokenExtractor)
		require.False(t, output.requireToken)
	})

	t.Run("Should apply WithTokenExtractor option", func(t *testing.T) {
		te := &testExtractor{}

		output, err := NewHttpAuthorizer("true", WithTokenExtractor(te, true))
		require.NoError(t, err)

		require.Equal(t, TokenFormatJWT, output.authTokenFormat)
		require.Equal(t, te, output.tokenExtractor)
		require.True(t, output.requireToken)
	})
}

func TestHttpAuthorizerCheck(t *testing.T) {
	cases := []struct {
		name         string
		rule         string
		method       string
		httpMethod   string
		URL          string
		md           map[string][]string
		tls          *x509.Certificate
		token        *oidc.IDToken
		requireToken bool
		tokenError   string
		expected     bool
		theError     string
	}{
		// http based attribute tests
		{
			name:       "Should allow if service and method match rule",
			rule:       "request.http_method == \"POST\" && request.method == \"MyMethod\"",
			method:     "MyMethod",
			httpMethod: "POST",
			URL:        "/",
			expected:   true,
		},
		{
			name:       "Should deny if method does not match",
			rule:       "request.http_method == \"POST\" && request.method == \"MyOtherMethod\"",
			method:     "MyMethod",
			httpMethod: "POST",
			URL:        "/",
			expected:   false,
			theError:   "policy denied",
		},
		{
			name:       "Should deny if URL does not match",
			rule:       "request.http_method == \"POST\" && request.path == \"/command\"",
			method:     "MyMethod",
			httpMethod: "POST",
			URL:        "/",
			expected:   false,
			theError:   "policy denied",
		},
		// TLS based attribute tests
		{
			name:       "Should allow if tls expression matches",
			rule:       "request.http_method == \"GET\" && request.tls.subject.common_name == \"my name\"",
			httpMethod: "GET",
			URL:        "/",
			tls:        makeCert(t, "my name"),
			expected:   true,
		},
		{
			name:       "Should deny if tls expression does not match",
			rule:       "request.http_method == \"GET\" && request.tls.subject.common_name == \"my other name\"",
			httpMethod: "GET",
			URL:        "/",
			tls:        makeCert(t, "my name"),
			expected:   false,
			theError:   "policy denied",
		},
		{
			name:       "Should deny if expression mentions tls but tls is nil",
			rule:       "request.http_method == \"GET\" && request.tls.subject.common_name == \"my other name\"",
			httpMethod: "GET",
			URL:        "/",
			tls:        nil,
			expected:   false,
			theError:   "policy denied",
		},
		// Metadata/header based attribute tests
		{
			name:       "Should allow if metadata expression matches",
			rule:       "request.http_method == \"GET\" && \"my-value\" in request.headers[\"My-Header\"].value",
			httpMethod: "GET",
			URL:        "/",
			md:         map[string][]string{"My-Header": {"not-my-value", "my-value"}},
			expected:   true,
		},
		{
			name:       "Should deny if metadata expression does not match",
			rule:       "request.http_method == \"GET\" && \"other-value\" in request.headers[\"My-Header\"].value",
			httpMethod: "GET",
			URL:        "/",
			md:         map[string][]string{"My-Header": {"not-my-value", "my-value"}},
			expected:   false,
			theError:   "policy denied",
		},
		{
			name:       "Should deny metadata expression if metadata is not present",
			rule:       "request.http_method == \"GET\" && \"other-value\" in request.headers[\"My-Header\"].value",
			httpMethod: "GET",
			URL:        "/",
			expected:   false,
			theError:   "policy evaluation failed: no such key: My-Header",
		},
		// OIDC based attribute tests
		{
			name:       "Should allow if oidc expression matches",
			rule:       "request.http_method == \"GET\" && request.jwt.subject == \"user@exoscale.com\"",
			httpMethod: "GET",
			URL:        "/",
			token:      &oidc.IDToken{Subject: "user@exoscale.com"},
			expected:   true,
		},
		{
			name:       "Should deny if oidc expression does not match",
			rule:       "request.http_method == \"GET\" && request.jwt.subject == \"user@exoscale.com\"",
			httpMethod: "GET",
			URL:        "/",
			token:      &oidc.IDToken{Subject: "other.user@exoscale.com"},
			expected:   false,
			theError:   "policy denied",
		},
		{
			name:       "Should deny oidc expression if token is not present",
			rule:       "request.http_method == \"GET\" && request.jwt.subject == \"user@exoscale.com\"",
			httpMethod: "GET",
			URL:        "/",
			expected:   false,
			theError:   "policy denied",
		},
		{
			name:         "Should deny if token extraction fails with required option set",
			rule:         "request.http_method == \"GET\"",
			httpMethod:   "GET",
			URL:          "/",
			token:        &oidc.IDToken{},
			requireToken: true,
			tokenError:   "invalid signature",
			expected:     false,
			theError:     "failed to extract JWT: invalid signature",
		},
		{
			name:         "Should allow if token extraction fails when required option is not set",
			rule:         "request.http_method == \"GET\"",
			httpMethod:   "GET",
			URL:          "/",
			requireToken: false,
			tokenError:   "failed to extract token",
			expected:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			req, err := http.NewRequestWithContext(ctx, tc.httpMethod, tc.URL, nil)
			require.NoError(t, err)
			if tc.tls != nil {
				if req.TLS == nil {
					req.TLS = &tls.ConnectionState{}
				}
				req.TLS.PeerCertificates = []*x509.Certificate{tc.tls}
			}
			if tc.md != nil {
				req.Header = tc.md
			}

			opts := []AuthorizerOption{}
			if tc.token != nil {
				var te *testExtractor
				if tc.tokenError == "" {
					te = &testExtractor{token: tc.token}
				} else {
					te = &testExtractor{token: tc.token, theError: errors.New(tc.tokenError)}
				}
				opts = append(opts, WithTokenExtractor(te, tc.requireToken))
			}

			authorizer, err := NewHttpAuthorizer(tc.rule, opts...)
			require.NoError(t, err)

			output, err := authorizer.Check(req, tc.method)
			if tc.expected {
				require.NoError(t, err)
				require.True(t, output)
			} else {
				require.EqualError(t, err, tc.theError)
				require.False(t, output)
			}
		})
	}
}

func BenchmarkCelAuthorizerCheck(b *testing.B) {
	rule := "request.method == \"health\" || request.tls.subject.common_name == \"root-api.root-api.pod\""
	cert := makeCert(b, "root-api.root-api.pod")
	req, err := http.NewRequestWithContext(b.Context(), "GET", "/health", nil)
	require.NoError(b, err)
	req.TLS.PeerCertificates = []*x509.Certificate{cert}

	authorizer, err := NewHttpAuthorizer(rule)
	require.NoError(b, err)
	for i := 0; i < b.N; i++ {
		authorizer.Check(req, "WriteExtent") //nolint:errcheck
	}
}

func BenchmarkCelAuthorizerCheckConcurrent(b *testing.B) {
	rule := "request.method == \"health\" || request.tls.subject.common_name == \"root-api.root-api.pod\""
	cert := makeCert(b, "root-api.root-api.pod")
	authorizer, err := NewHttpAuthorizer(rule)
	require.NoError(b, err)

	b.RunParallel(func(pb *testing.PB) {
		req, err := http.NewRequestWithContext(b.Context(), "GET", "/health", nil)
		require.NoError(b, err)
		req.TLS.PeerCertificates = []*x509.Certificate{cert}
		for pb.Next() {
			authorizer.Check(req, "WriteExtent") //nolint:errcheck
		}
	})
}
