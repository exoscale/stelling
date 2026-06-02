package interceptor

import (
	"fmt"
	"net/http"

	"github.com/exoscale/stelling/fxauthorizer/schema"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/decls"
	"github.com/google/cel-go/common/types"
)

type httpAuthorizer struct {
	authorizer
}

// compileHttpCelProgram compiles the given expression in the context of an HttpRequest
func compileHttpCelProgram(rule string) (cel.Program, error) {
	env, err := cel.NewEnv(
		cel.Types(new(schema.HttpRequest)),
		cel.VariableDecls(decls.NewVariable("request", types.NewObjectType("exoscale.rpc.authorizer.v1.HttpRequest"))),
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

// NewHttpAuthorizer produces an Authorizer that can evaluate a CEL policy over Grpc requests
// The rule must evaluate to a bool
func NewHttpAuthorizer(rule string, opts ...authorizerOption) (*httpAuthorizer, error) {
	program, err := compileHttpCelProgram(rule)
	if err != nil {
		return nil, err
	}
	output := &httpAuthorizer{
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
func (a *httpAuthorizer) Check(r *http.Request, method string) (bool, error) {
	req := &schema.HttpRequest{
		HttpMethod: r.Method,
		Path:       r.URL.Path,
		Host:       r.Host,
		Scheme:     r.URL.Scheme,
		Query:      r.URL.RawQuery,
		Size:       r.ContentLength,
		Protocol:   r.Proto,
		Method:     method,
	}
	ctx := r.Context()

	req.Headers = schema.NewHeaders(r.Header)

	if a.authTokenFormat == TokenFormatJWT {
		token, err := a.tokenExtractor.Extract(ctx, r.Header)
		if err != nil && a.requireToken {
			return false, fmt.Errorf("failed to extract JWT: %w", err)
		}

		req.Jwt = schema.NewJWT(token)
	}

	// If no info, we'll continue and set nil for the TLS info
	if tlsInfo := r.TLS; tlsInfo != nil {
		if len(tlsInfo.PeerCertificates) != 0 {
			req.Tls = schema.NewTLS(tlsInfo.PeerCertificates[0])
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
