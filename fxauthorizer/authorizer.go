package fxauthorizer

import (
	"net/http"

	"github.com/exoscale/stelling/fxauthorizer/interceptor"
	"github.com/exoscale/stelling/fxgrpc"
	"github.com/exoscale/stelling/fxhttp"
	"go.uber.org/fx"
)

// NewModule provides authorization middleware to the system:
// * Grpc server interceptors
// * Http server middleware (TODO)
// Keep in mind that the Authorizer components for Grpc and Http are
// distinct, but share the same config.
// If you need different rules for either protocol, you must supply
// 2 different configurations with proper annotations to your system
func NewModule(conf AuthorizerConfig) fx.Option {
	return fx.Module(
		"authorizer",
		fx.Provide(
			NewGrpcAuthorizer,
			NewHttpAuthorizer,
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

func NewGrpcAuthorizer(conf AuthorizerConfig) (interceptor.GrpcAuthorizer, error) {
	return interceptor.NewGrpcAuthorizer(conf.AuthorizerConfig().Rule)
}

func NewHttpAuthorizer(conf AuthorizerConfig) (interceptor.HttpAuthorizer, error) {
	return interceptor.NewHttpAuthorizer(conf.AuthorizerConfig().Rule)
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
