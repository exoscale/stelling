package interceptor

import (
	"context"
	"fmt"

	"github.com/exoscale/stelling/fxauthorizer/schema"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/decls"
	"github.com/google/cel-go/common/types"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

type grpcAuthorizer struct {
	authorizer
}

// compileGrpcCelProgram compiles the given expression in the context of a GrpcRequest
func compileGrpcCelProgram(rule string) (cel.Program, error) {
	env, err := cel.NewEnv(
		cel.Types(new(schema.GrpcRequest)),
		cel.VariableDecls(decls.NewVariable("request", types.NewObjectType("exoscale.rpc.authorizer.v1.GrpcRequest"))),
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

// NewGrpcAuthorizer produces an Authorizer that can evaluate a CEL policy over Grpc requests
// The rule must evaluate to a bool
func NewGrpcAuthorizer(rule string, opts ...AuthorizerOption) (*grpcAuthorizer, error) {
	program, err := compileGrpcCelProgram(rule)
	if err != nil {
		return nil, err
	}
	output := &grpcAuthorizer{
		authorizer: authorizer{
			authTokenFormat: TokenFormatNone,
			rule:            program,
		},
	}
	for _, opt := range opts {
		opt(&output.authorizer)
	}
	return output, nil
}

// Check evaluates the configured policy over a request
// If the check fails, the error will contain detailed information about why the evaluation failed
func (a *grpcAuthorizer) Check(ctx context.Context, service string, method string) (bool, error) {
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
