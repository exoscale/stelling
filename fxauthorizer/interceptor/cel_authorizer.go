package interceptor

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/exoscale/stelling/fxauthorizer/schema"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/types"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
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

type celAuthorizer struct {
	authTokenFormat TokenFormat
	rule            cel.Program
	tokenExtractor  TokenExtractor
	requireToken    bool
}

type celAuthorizerOption func(*celAuthorizer)

// WithTokenExtractor will populate the request.jwt field with the IDToken produced by the extractor
// If requireToken is set, the request will be denied if token extraction fails, without evaluating the policy
// If requireToken is false, JWT will be nil if token extraction fails and the policy will be evaluated
func WithTokenExtractor(te TokenExtractor, requireToken bool) celAuthorizerOption {
	return func(ca *celAuthorizer) {
		ca.authTokenFormat = TokenFormatJWT
		ca.tokenExtractor = te
		ca.requireToken = requireToken
	}
}

// compileCelProgram compiles the given expression in the context of a GrpcRequest
func compileCelProgram(rule string) (cel.Program, error) {
	env, err := cel.NewEnv(
		cel.Types(new(schema.GrpcRequest)),
		cel.Declarations(decls.NewVar("request", decls.NewObjectType("exoscale.rpc.authorizer.v1.GrpcRequest"))),
	)
	if err != nil {
		return nil, err
	}
	ast, issues := env.Compile(rule)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	return env.Program(ast)
}

// NewCelAuthorizer produces an Authorizer that can evaluate a CEL policy over Grpc requests
// The rule must evaluate to a bool
func NewCelAuthorizer(rule string, opts ...celAuthorizerOption) (*celAuthorizer, error) {
	program, err := compileCelProgram(rule)
	if err != nil {
		return nil, err
	}
	output := &celAuthorizer{
		authTokenFormat: TokenFormatNone,
		rule:            program,
	}
	for _, opt := range opts {
		opt(output)
	}
	return output, nil
}

// Check evaluates the configured policy over a request
// If the check fails, the error will contain detailed information about why the evaluation failed
func (a *celAuthorizer) Check(ctx context.Context, service string, method string) (bool, error) {
	req := &schema.GrpcRequest{
		Service: service,
		Method:  method,
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		req.Headers = schema.NewHeaders(md)
	}

	if a.authTokenFormat == TokenFormatJWT {
		token, err := a.tokenExtractor.Extract(ctx, md)
		if err != nil && a.requireToken {
			return false, fmt.Errorf("failed to extract JWT: %w", err)
		}

		req.Jwt = schema.NewJWT(token)
	}

	peerInfo, ok := peer.FromContext(ctx)
	// If no info, we'll continue and set nil for the TLS info
	if ok {
		tlsInfo, ok := peerInfo.AuthInfo.(credentials.TLSInfo)
		if ok {
			if len(tlsInfo.State.PeerCertificates) != 0 {
				req.Tls = schema.NewTLS(tlsInfo.State.PeerCertificates[0])
			}
		}
	}

	out, _, err := a.rule.ContextEval(ctx, map[string]any{"request": req})
	if err != nil {
		return false, fmt.Errorf("policy evaluation failed: %w", err)
	}

	if out == types.Bool(true) {
		return true, nil
	} else {
		return false, fmt.Errorf("policy denied")
	}
}
