package oidc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewOIDCVerifier(t *testing.T) {
	server, _ := setupOIDCTest(t, map[string]map[string]string{})

	cases := []struct {
		name     string
		url      string
		clientID string
		isError  bool
		errMsg   string
	}{
		{
			name:    "Should return an error if the issuerURL is empty",
			isError: true,
			errMsg:  "jwtIssuerURL must not be empty",
		},
		{
			name:    "Should return an error if the issuerURL cannot be contacted",
			url:     "http://i.am.not.a.valid.server",
			isError: true,
			errMsg:  "no such host",
		},
		{
			name:     "Should return a TokenVerifier if the config is valid",
			url:      server.URL,
			clientID: "my_client_id",
			isError:  false,
		},
		{
			name:    "Should return a TokenVerifier even if no client_id is given",
			url:     server.URL,
			isError: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := newOIDCVerifier(tc.url, tc.clientID, false)
			if tc.isError {
				if tc.errMsg != "" {
					require.Contains(t, err.Error(), tc.errMsg)
				} else {
					require.Error(t, err)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, output)
			}
		})
	}
}

func TestNewTokenExtractor(t *testing.T) {
	t.Run("Should return an error if the issuerURL cannot be contacted", func(t *testing.T) {
		te, err := NewTokenExtractor("not.a.valid.server", "client_id")
		require.Nil(t, te)
		require.Error(t, err)
	})

	t.Run("Should return a TokenExtractor with correct default options", func(t *testing.T) {
		server, _ := setupOIDCTest(t, map[string]map[string]string{})

		te, err := NewTokenExtractor(server.URL, "client_id")
		require.NoError(t, err)

		require.Equal(t, "Authorization", te.header)
		require.Equal(t, "client_id", te.clientID)
		require.NotNil(t, te.verifier)
		require.False(t, te.skipClientIDCheck)
	})

	t.Run("Should apply WithAuthHeader option", func(t *testing.T) {
		server, _ := setupOIDCTest(t, map[string]map[string]string{})

		te, err := NewTokenExtractor(server.URL, "client_id", WithAuthHeader("My-Header"))
		require.NoError(t, err)

		require.Equal(t, "My-Header", te.header)
	})

	t.Run("Should apply WithSkipClientIDCheck option", func(t *testing.T) {
		server, _ := setupOIDCTest(t, map[string]map[string]string{})

		te, err := NewTokenExtractor(server.URL, "client_id", WithSkipClientIDCheck())
		require.NoError(t, err)

		require.Empty(t, te.clientID)
		require.True(t, te.skipClientIDCheck)
	})

	t.Run("Should apply all given options", func(t *testing.T) {
		server, _ := setupOIDCTest(t, map[string]map[string]string{})

		te, err := NewTokenExtractor(server.URL, "client_id", WithAuthHeader("My-Header"), WithSkipClientIDCheck())
		require.NoError(t, err)

		require.Equal(t, "My-Header", te.header)
		require.Empty(t, te.clientID)
		require.True(t, te.skipClientIDCheck)
	})
}

func NewSimpleTestExtractor(t *testing.T) *TokenExtractor {
	t.Helper()
	server, _ := setupOIDCTest(t, map[string]map[string]string{})
	te, err := NewTokenExtractor(server.URL, "")
	require.NoError(t, err)
	return te
}

func TestTokenExtractorExtract(t *testing.T) {
	t.Run("Should return an error if the metadata is nil", func(t *testing.T) {
		te := NewSimpleTestExtractor(t)

		token, err := te.Extract(context.Background(), nil)
		require.Nil(t, token)
		require.EqualError(t, err, "no metadata to extract token from")
	})

	t.Run("Should return an error if the authorization header is not present", func(t *testing.T) {
		te := NewSimpleTestExtractor(t)

		token, err := te.Extract(context.Background(), map[string][]string{})
		require.Nil(t, token)
		require.EqualError(t, err, "authorization header 'Authorization' is missing")
	})

	t.Run("Should return an error if the authorization header is malformatted", func(t *testing.T) {
		te := NewSimpleTestExtractor(t)

		token, err := te.Extract(context.Background(), map[string][]string{
			"Authorization": {"foobar"},
		})
		require.Nil(t, token)
		require.EqualError(t, err, "malformed authorization header")
	})

	t.Run("Should return an error if the token cannot be parsed", func(t *testing.T) {
		te := NewSimpleTestExtractor(t)

		token, err := te.Extract(context.Background(), map[string][]string{
			"Authorization": {"Bearer foobar"},
		})
		require.Nil(t, token)
		require.EqualError(t, err, "invalid token: oidc: malformed jwt: oidc: malformed jwt, expected 3 parts got 1")
	})

	t.Run("Should return an error if the token fails validation", func(t *testing.T) {
		te := NewSimpleTestExtractor(t)

		key, err := newRSAKey()
		require.NoError(t, err)
		token2, err := key.createIdToken(
			"https://some.other.server",
			"J. Doe",
			"jdoe@example.com",
			[]string{"ops", "dev"},
		)
		require.NoError(t, err)

		token, err := te.Extract(context.Background(), map[string][]string{
			"Authorization": {"Bearer " + token2},
		})
		require.Nil(t, token)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid token: oidc: id token issued by a different provider")
	})

	t.Run("Should return an error if the token fails signature validation", func(t *testing.T) {
		server, _ := setupOIDCTest(t, map[string]map[string]string{})

		key, err := newRSAKey()
		require.NoError(t, err)
		token2, err := key.createIdToken(
			server.URL,
			"J. Doe",
			"jdoe@example.com",
			[]string{"ops", "dev"},
		)
		require.NoError(t, err)

		te, err := NewTokenExtractor(server.URL, "idtest")
		require.NoError(t, err)

		token, err := te.Extract(context.Background(), map[string][]string{
			"Authorization": {"Bearer " + token2},
		})
		require.Nil(t, token)
		require.EqualError(t, err, "invalid token: failed to verify signature: failed to verify id token signature")
	})

	t.Run("Should return a parsed token", func(t *testing.T) {
		server, key := setupOIDCTest(t, map[string]map[string]string{})
		token, err := key.createIdToken(
			server.URL,
			"J. Doe",
			"jdoe@example.com",
			[]string{"ops", "dev"},
		)
		require.NoError(t, err)
		te, err := NewTokenExtractor(server.URL, "idtest")
		require.NoError(t, err)

		parsedToken, err := te.Extract(context.Background(), map[string][]string{
			"Authorization": {"Bearer " + token},
		})
		require.NoError(t, err)
		require.NotNil(t, parsedToken)
	})

	t.Run("Should return a parsed token from the configured header", func(t *testing.T) {
		header := "Other-Header"
		server, key := setupOIDCTest(t, map[string]map[string]string{})
		token, err := key.createIdToken(
			server.URL,
			"Jane Doe",
			"janedoe@example.com",
			[]string{"ops", "dev", "admin"},
		)
		require.NoError(t, err)

		te, err := NewTokenExtractor(server.URL, "idtest", WithAuthHeader(header))
		require.NoError(t, err)

		parsedToken, err := te.Extract(context.Background(), map[string][]string{
			header: {"Bearer " + token},
		})
		require.NoError(t, err)
		require.NotNil(t, parsedToken)
	})
}
