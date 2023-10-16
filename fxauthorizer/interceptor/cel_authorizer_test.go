package interceptor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

func TestParseTokenFormat(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected TokenFormat
		isError  bool
	}{
		{
			name:     "Should parse TokenFormatJWT",
			input:    "jwt",
			expected: TokenFormatJWT,
		},
		{
			name:     "Should parse TokenFormatNone",
			input:    "none",
			expected: TokenFormatNone,
		},
		{
			name:     "Should ignore case when parsing",
			input:    "JwT",
			expected: TokenFormatJWT,
		},
		{
			name:     "Should return an error if the input not a TokenFormat",
			input:    "notaformat",
			isError:  true,
			expected: TokenFormatInvalid,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := ParseTokenFormat(tc.input)
			if tc.isError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expected, output)
		})
	}
}

func TestCompileCelProgram(t *testing.T) {
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
			output, err := compileCelProgram(tc.input)
			if tc.isError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, output)
			}
		})
	}
}

type testExtractor struct {
	token    *oidc.IDToken
	theError error
}

func (te *testExtractor) Extract(ctx context.Context, md map[string][]string) (*oidc.IDToken, error) {
	if te.theError != nil {
		return nil, te.theError
	}
	return te.token, nil
}

func TestNewCelAuthorizer(t *testing.T) {
	t.Run("Should return an error when the CEL rule is invalid", func(t *testing.T) {
		output, err := NewCelAuthorizer("foobar")
		require.Nil(t, output)
		require.Error(t, err)
	})

	t.Run("Should return an authorizer with correct defaults", func(t *testing.T) {
		output, err := NewCelAuthorizer("true")
		require.NoError(t, err)

		require.Equal(t, TokenFormatNone, output.authTokenFormat)
		require.NotNil(t, output.rule)
		require.Nil(t, output.tokenExtractor)
		require.False(t, output.requireToken)
	})

	t.Run("Should apply WithTokenExtractor option", func(t *testing.T) {
		te := &testExtractor{}

		output, err := NewCelAuthorizer("true", WithTokenExtractor(te, true))
		require.NoError(t, err)

		require.Equal(t, TokenFormatJWT, output.authTokenFormat)
		require.Equal(t, te, output.tokenExtractor)
		require.True(t, output.requireToken)
	})
}

func makeCert(tb testing.TB, name string) *x509.Certificate {
	tb.Helper()

	return &x509.Certificate{
		Version:      1,
		SerialNumber: big.NewInt(2023),
		Issuer: pkix.Name{
			Organization:       []string{"Exoscale"},
			OrganizationalUnit: []string{},
			Country:            []string{"CH"},
			Province:           []string{},
			Locality:           []string{"Vaud"},
			StreetAddress:      []string{"Lake Street"},
			PostalCode:         []string{"12345"},
			CommonName:         "Exoscale",
		},
		Subject: pkix.Name{
			Organization:       []string{},
			OrganizationalUnit: []string{},
			Country:            []string{},
			Province:           []string{},
			Locality:           []string{},
			StreetAddress:      []string{},
			PostalCode:         []string{},
			CommonName:         name,
		},
	}
}

func TestCelAuthorizerCheck(t *testing.T) {
	cases := []struct {
		name         string
		rule         string
		service      string
		method       string
		md           map[string][]string
		tls          *x509.Certificate
		token        *oidc.IDToken
		requireToken bool
		tokenError   string
		expected     bool
		theError     string
	}{
		// gRPC based attribute tests
		{
			name:     "Should allow if service and method match rule",
			rule:     "request.service == \"MyService\" && request.method == \"MyMethod\"",
			service:  "MyService",
			method:   "MyMethod",
			expected: true,
		},
		{
			name:     "Should deny if method does not match",
			rule:     "request.service == \"MyService\" && request.method == \"MyOtherMethod\"",
			service:  "MyService",
			method:   "MyMethod",
			expected: false,
			theError: "policy denied",
		},
		// TLS based attribute tests
		{
			name:     "Should allow if tls expression matches",
			rule:     "request.service == \"MyService\" && request.tls.subject.common_name == \"my name\"",
			service:  "MyService",
			tls:      makeCert(t, "my name"),
			expected: true,
		},
		{
			name:     "Should deny if tls expression does not match",
			rule:     "request.service == \"MyService\" && request.tls.subject.common_name == \"my other name\"",
			service:  "MyService",
			tls:      makeCert(t, "my name"),
			expected: false,
			theError: "policy denied",
		},
		{
			name:     "Should deny if expression mentions tls but tls is nil",
			rule:     "request.service == \"MyService\" && request.tls.subject.common_name == \"my other name\"",
			service:  "MyService",
			tls:      nil,
			expected: false,
			theError: "policy denied",
		},
		// Metadata/header based attribute tests
		{
			name:     "Should allow if metadata expression matches",
			rule:     "request.service == \"MyService\" && \"my-value\" in request.headers[\"My-Header\"].value",
			service:  "MyService",
			md:       map[string][]string{"My-Header": {"not-my-value", "my-value"}},
			expected: true,
		},
		{
			name:     "Should deny if metadata expression does not match",
			rule:     "request.service == \"MyService\" && \"other-value\" in request.headers[\"My-Header\"].value",
			service:  "MyService",
			md:       map[string][]string{"My-Header": {"not-my-value", "my-value"}},
			expected: false,
			theError: "policy denied",
		},
		{
			name:     "Should deny metadata expression if metadata is not present",
			rule:     "request.service == \"MyService\" && \"other-value\" in request.headers[\"My-Header\"].value",
			service:  "MyService",
			expected: false,
			theError: "policy evaluation failed: no such key: My-Header",
		},
		// OIDC based attribute tests
		{
			name:     "Should allow if oidc expression matches",
			rule:     "request.service == \"MyService\" && request.jwt.subject == \"user@exoscale.com\"",
			service:  "MyService",
			token:    &oidc.IDToken{Subject: "user@exoscale.com"},
			expected: true,
		},
		{
			name:     "Should deny if oidc expression does not match",
			rule:     "request.service == \"MyService\" && request.jwt.subject == \"user@exoscale.com\"",
			service:  "MyService",
			token:    &oidc.IDToken{Subject: "other.user@exoscale.com"},
			expected: false,
			theError: "policy denied",
		},
		{
			name:     "Should deny oidc expression if token is not present",
			rule:     "request.service == \"MyService\" && request.jwt.subject == \"user@exoscale.com\"",
			service:  "MyService",
			expected: false,
			theError: "policy denied",
		},
		{
			name:         "Should deny if token extraction fails with required option set",
			rule:         "request.service == \"MyService\"",
			service:      "MyService",
			token:        &oidc.IDToken{},
			requireToken: true,
			tokenError:   "invalid signature",
			expected:     false,
			theError:     "failed to extract JWT: invalid signature",
		},
		{
			name:         "Should allow if token extraction fails when required option is not set",
			rule:         "request.service == \"MyService\"",
			service:      "MyService",
			requireToken: false,
			tokenError:   "failed to extract token",
			expected:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.tls != nil {
				authInfo := credentials.TLSInfo{State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{tc.tls}}}
				ctx = peer.NewContext(ctx, &peer.Peer{AuthInfo: authInfo})
			}
			if tc.md != nil {
				ctx = metadata.NewIncomingContext(ctx, tc.md)
			}

			opts := []celAuthorizerOption{}
			if tc.token != nil {
				var te *testExtractor
				if tc.tokenError == "" {
					te = &testExtractor{token: tc.token}
				} else {
					te = &testExtractor{token: tc.token, theError: errors.New(tc.tokenError)}
				}
				opts = append(opts, WithTokenExtractor(te, tc.requireToken))
			}

			authorizer, err := NewCelAuthorizer(tc.rule, opts...)
			require.NoError(t, err)

			output, err := authorizer.Check(ctx, tc.service, tc.method)
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
	rule := "request.service == \"gprc.health.v1.Health\" || request.tls.subject.common_name == \"root-api.root-api.pod\""
	cert := makeCert(b, "root-api.root-api.pod")
	authInfo := credentials.TLSInfo{State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}}
	ctx := peer.NewContext(context.Background(), &peer.Peer{AuthInfo: authInfo})

	authorizer, err := NewCelAuthorizer(rule)
	require.NoError(b, err)
	for i := 0; i < b.N; i++ {
		authorizer.Check(ctx, "ExtentService", "WriteExtent") //nolint:errcheck
	}
}
