package fxauthorizer

import (
	"net/http"

	"github.com/exoscale/stelling/fxauthorizer/interceptor"
	"github.com/exoscale/stelling/fxauthorizer/oidc"
	"github.com/exoscale/stelling/fxgrpc"
	"github.com/exoscale/stelling/fxhttp"
	"go.uber.org/fx"
)

type moduleOpts struct {
	requireToken bool
}

type moduleOption func(*moduleOpts)

// WithRequireJWT sets whether the presence of a JWT in the Authorization header is required for auth-z
func WithRequireJWT(requireToken bool) moduleOption {
	return func(o *moduleOpts) {
		o.requireToken = requireToken
	}
}

// NewModule provides authorization middleware to the system:
// * Grpc server interceptors
// * Http server middleware
// Keep in mind that the Authorizer components for Grpc and Http are
// distinct, but share the same config.
// If you need different rules for either protocol, you must supply
// 2 different configurations with proper annotations to your system
func NewModule(conf AuthorizerConfig, modOpts ...moduleOption) fx.Option {
	options := &moduleOpts{}
	for _, o := range modOpts {
		o(options)
	}

	opts := fx.Options(
		fx.Provide(
			fx.Annotate(NewGrpcAuthorizer, fx.ParamTags(``, `group:"authorizer_option"`)),
			fx.Annotate(NewHttpAuthorizer, fx.ParamTags(``, `group:"authorizer_option"`)),
			fx.Annotate(
				NewGrpcAuthorizerServerInterceptors,
				fx.ResultTags(`group:"unary_server_interceptor"`, `group:"stream_server_interceptor"`),
			),
			NewHttpMiddleware,
		),
		fx.Supply(
			fx.Annotate(conf, fx.As(new(AuthorizerConfig))),
			tokenRequired(options.requireToken),
			fx.Private,
		),
	)

	if conf.AuthorizerConfig().IdpEndpoint != "" {
		opts = fx.Options(
			opts,
			fx.Provide(
				fx.Annotate(newTokenExtractor, fx.ResultTags(`group:"authorizer_option"`)),
			),
		)
	}

	return fx.Module(
		"authorizer",
		opts,
	)
}

type AuthorizerConfig interface {
	AuthorizerConfig() *Authorizer
}

// Logging contains the configuration options for the authorizer module
type Authorizer struct {
	// The CEL expression that will be evaluated for each request made to the server
	Rule string `validate:"required"`
	// The URL where we can find the IdP that signs trusted JWTs
	IdpEndpoint string `validate:"url"`
}

func (a *Authorizer) AuthorizerConfig() *Authorizer {
	return a
}

type tokenRequired bool

func newTokenExtractor(conf AuthorizerConfig, tokenRequired tokenRequired) (interceptor.AuthorizerOption, error) {
	extractor, err := oidc.NewTokenExtractor(conf.AuthorizerConfig().IdpEndpoint, "", oidc.WithSkipClientIDCheck())
	if err != nil {
		return nil, err
	}
	return interceptor.WithTokenExtractor(extractor, bool(tokenRequired)), nil
}

func NewGrpcAuthorizer(conf AuthorizerConfig, opts ...interceptor.AuthorizerOption) (interceptor.GrpcAuthorizer, error) {
	return interceptor.NewGrpcAuthorizer(conf.AuthorizerConfig().Rule, opts...)
}

func NewHttpAuthorizer(conf AuthorizerConfig, opts ...interceptor.AuthorizerOption) (interceptor.HttpAuthorizer, error) {
	return interceptor.NewHttpAuthorizer(conf.AuthorizerConfig().Rule, opts...)
}

// Setting this late in the chain so observability interceptors can monitor requests that fail authorization
const GrpcInterceptorWeight uint = 70

func NewGrpcAuthorizerServerInterceptors(a interceptor.GrpcAuthorizer) (*fxgrpc.UnaryServerInterceptor, *fxgrpc.StreamServerInterceptor) {
	unaryIx := &fxgrpc.UnaryServerInterceptor{Weight: GrpcInterceptorWeight, Interceptor: interceptor.NewAuthorizerUnaryServerInterceptor(a)}
	streamIx := &fxgrpc.StreamServerInterceptor{Weight: GrpcInterceptorWeight, Interceptor: interceptor.NewAuthorizerStreamServerInterceptor(a)}
	return unaryIx, streamIx
}

type HttpMiddlewareResult struct {
	fx.Out

	Middleware *fxhttp.Middleware `group:"http_middleware"`
}

func NewHttpMiddleware(authorizer interceptor.HttpAuthorizer) HttpMiddlewareResult {
	return HttpMiddlewareResult{
		Middleware: &fxhttp.Middleware{
			Handler: func(next http.Handler) http.Handler {
				return interceptor.NewAuthorizerHandler(authorizer, next)
			},
			Weight: int(GrpcInterceptorWeight),
		},
	}
}
