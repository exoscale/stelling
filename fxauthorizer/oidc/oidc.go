package oidc

import (
	"context"
	"fmt"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
)

type TokenExtractor struct {
	header            string
	verifier          *oidc.IDTokenVerifier
	clientID          string
	skipClientIDCheck bool
}

type tokenExtractorOption func(*TokenExtractor)

// WithSkipClientIDCheck will disable the client_id check validation when parsing jwt tokens
func WithSkipClientIDCheck() tokenExtractorOption {
	return func(te *TokenExtractor) {
		te.clientID = ""
		te.skipClientIDCheck = true
	}
}

// WithAuthHeader sets a custom header to read the jwt token from
// By default the 'Authorization' header is used
func WithAuthHeader(header string) tokenExtractorOption {
	return func(te *TokenExtractor) {
		te.header = header
	}
}

// NewTokenExtractor produces a TokenExtractor that can extract, verify and parse oidc IDTokens from request headers
func NewTokenExtractor(jwtIssuerURL string, clientID string, opts ...tokenExtractorOption) (*TokenExtractor, error) {
	te := &TokenExtractor{
		header:   http.CanonicalHeaderKey("Authorization"),
		clientID: clientID,
	}

	for _, opt := range opts {
		opt(te)
	}

	verifier, err := newOIDCVerifier(jwtIssuerURL, te.clientID, te.skipClientIDCheck)
	if err != nil {
		return nil, err
	}

	te.verifier = verifier
	return te, nil
}

func newOIDCVerifier(jwtIssuerURL string, jwtClientID string, skipClientIDCheck bool) (*oidc.IDTokenVerifier, error) {
	if jwtIssuerURL == "" {
		return nil, fmt.Errorf("jwtIssuerURL must not be empty")
	}
	// Using context.Background because the resulting Verifier is tied to the lifetime of the context
	// Since you'll call this at init time, you'd typically have an fx Start context at hand, which
	// does not live long enough.
	oidcProvider, err := oidc.NewProvider(context.Background(), jwtIssuerURL)
	if err != nil {
		return nil, err
	}
	oidcConfig := &oidc.Config{
		ClientID:          jwtClientID,
		SkipClientIDCheck: skipClientIDCheck,
	}
	return oidcProvider.Verifier(oidcConfig), nil
}

func (te *TokenExtractor) Extract(ctx context.Context, md map[string][]string) (*oidc.IDToken, error) {
	if md == nil {
		return nil, fmt.Errorf("no metadata to extract token from")
	}
	authHeader := md[te.header]
	if len(authHeader) == 0 {
		return nil, fmt.Errorf("authorization header '%s' is missing", te.header)
	}
	var token string
	n, err := fmt.Sscanf(authHeader[0], "Bearer %s", &token)
	if err != nil || n != 1 {
		return nil, fmt.Errorf("malformed authorization header")
	}
	parsedToken, err := te.verifier.Verify(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	return parsedToken, nil
}
