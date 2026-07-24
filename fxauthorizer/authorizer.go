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
	tokenExtractorConfig  *tokenExtractorConfig
	tokenExtractorOptions []oidc.TokenExtractorOption
}

type moduleOption func(*moduleOpts)

// WithTokenExtractor will populate the request.jwt field with the IDToken produced by an
// oidc.TokenExtractor built from the given identity provider URL and client ID.
// If requireToken is set, the request will be denied if token extraction fails, without evaluating the policy
// If requireToken is false, JWT will be nil if token extraction fails and the policy will be evaluated
func WithTokenExtractor(requireToken bool, identityProviderUrl string, clientId string, opts ...oidc.TokenExtractorOption) moduleOption {
	return func(o *moduleOpts) {
		o.tokenExtractorConfig = &tokenExtractorConfig{
			identityProviderUrl: identityProviderUrl,
			clientId:            clientId,
			requireToken:        requireToken,
		}
		o.tokenExtractorOptions = opts
	}
}

// tokenExtractorConfig bundles the parameters needed to build the oidc.TokenExtractor so they can
// be threaded through fx as a single optional dependency. NewGrpcAuthorizer/NewHttpAuthorizer each
// build their own extractor instance from it. The TokenExtractorOptions travel separately, through
// the authorizer_oidc_token_extractor_option group, and are collected back up via ParamTags.
type tokenExtractorConfig struct {
	identityProviderUrl string
	clientId            string
	requireToken        bool
}

// NewModule provides authorization middleware to the system:
// * Grpc server interceptors
// * Http server middleware
// Keep in mind that the Authorizer components for Grpc and Http are
// distinct, but share the same config.
// If you need different rules for either protocol, you must supply
// 2 different configurations with proper annotations to your system
func NewModule(conf AuthorizerConfig, opts ...moduleOption) fx.Option {
	modOpts := &moduleOpts{}
	for _, o := range opts {
		o(modOpts)
	}

	supplyOpts := fx.Options()
	if modOpts.tokenExtractorConfig != nil {
		supplyOpts = fx.Options(
			fx.Supply(
				fx.Private,
				modOpts.tokenExtractorConfig,
				// Threading the TokenExtractorOptions through a group lets us hand them to the
				// variadic teOpts parameter of NewGrpcAuthorizer/NewHttpAuthorizer below via ParamTags
				fx.Annotate(
					modOpts.tokenExtractorOptions,
					fx.ResultTags(`group:"authorizer_oidc_token_extractor_option,flatten"`),
				),
			),
		)
	}

	return fx.Module(
		"authorizer",
		supplyOpts,
		fx.Provide(
			fx.Annotate(NewGrpcAuthorizer, fx.ParamTags(``, `optional:"true"`, `group:"authorizer_oidc_token_extractor_option"`)),
			fx.Annotate(NewHttpAuthorizer, fx.ParamTags(``, `optional:"true"`, `group:"authorizer_oidc_token_extractor_option"`)),
			fx.Annotate(
				NewGrpcAuthorizerServerInterceptors,
				fx.ResultTags(`group:"unary_server_interceptor"`, `group:"stream_server_interceptor"`),
			),
			NewHttpMiddleware,
		),
		fx.Supply(
			fx.Annotate(conf, fx.As(new(AuthorizerConfig))),
			fx.Private,
		),
	)
}

type AuthorizerConfig interface {
	AuthorizerConfig() *Authorizer
}

// Logging contains the configuration options for the authorizer module
type Authorizer struct {
	// The CEL expression that will be evaluated for each request made to the server
	Rule string `validate:"required"`
	// TODO: Add oidc options when we need them
}

func (a *Authorizer) AuthorizerConfig() *Authorizer {
	return a
}

func NewGrpcAuthorizer(conf AuthorizerConfig, te *tokenExtractorConfig, teOpts ...oidc.TokenExtractorOption) (interceptor.GrpcAuthorizer, error) {
	if te == nil {
		return interceptor.NewGrpcAuthorizer(conf.AuthorizerConfig().Rule)
	}
	extractor, err := oidc.NewTokenExtractor(te.identityProviderUrl, te.clientId, teOpts...)
	if err != nil {
		return nil, err
	}
	return interceptor.NewGrpcAuthorizer(conf.AuthorizerConfig().Rule, interceptor.WithTokenExtractor(extractor, te.requireToken))
}

func NewHttpAuthorizer(conf AuthorizerConfig, te *tokenExtractorConfig, teOpts ...oidc.TokenExtractorOption) (interceptor.HttpAuthorizer, error) {
	if te == nil {
		return interceptor.NewHttpAuthorizer(conf.AuthorizerConfig().Rule)
	}
	extractor, err := oidc.NewTokenExtractor(te.identityProviderUrl, te.clientId, teOpts...)
	if err != nil {
		return nil, err
	}
	return interceptor.NewHttpAuthorizer(conf.AuthorizerConfig().Rule, interceptor.WithTokenExtractor(extractor, te.requireToken))
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
