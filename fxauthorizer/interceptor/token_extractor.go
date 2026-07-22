package interceptor

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/cel-go/cel"
)

type TokenFormat int

const (
	TokenFormatInvalid TokenFormat = iota
	TokenFormatNone
	TokenFormatJWT
)

func ParseTokenFormat(input string) (TokenFormat, error) {
	switch strings.ToLower(input) {
	case "none":
		return TokenFormatNone, nil
	case "jwt":
		return TokenFormatJWT, nil
	default:
		return TokenFormatInvalid, fmt.Errorf("cannot parse TokenFormat: %s", input)
	}
}

type TokenExtractor interface {
	// Extract returns a parsed IDToken from a set of request headers
	Extract(ctx context.Context, md map[string][]string) (*oidc.IDToken, error)
}

type authorizer struct {
	authTokenFormat TokenFormat
	rule            cel.Program
	tokenExtractor  TokenExtractor
	requireToken    bool
}

type authorizerOption func(*authorizer)

// WithTokenExtractor will populate the request.jwt field with the IDToken produced by the extractor
// If requireToken is set, the request will be denied if token extraction fails, without evaluating the policy
// If requireToken is false, JWT will be nil if token extraction fails and the policy will be evaluated
func WithTokenExtractor(te TokenExtractor, requireToken bool) authorizerOption {
	return func(ca *authorizer) {
		ca.authTokenFormat = TokenFormatJWT
		ca.tokenExtractor = te
		ca.requireToken = requireToken
	}
}
