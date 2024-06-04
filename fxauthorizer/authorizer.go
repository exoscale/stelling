package fxauthorizer

import (
	"github.com/exoscale/stelling/fxauthorizer/interceptor"
	"github.com/exoscale/stelling/fxgrpc"
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
			NewAuthorizer,
			fx.Annotate(
				NewGrpcAuthorizerServerInterceptors,
				fx.ResultTags(`group:"unary_server_interceptor"`, `group:"stream_server_interceptor"`),
			),
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

func NewAuthorizer(conf AuthorizerConfig) (interceptor.Authorizer, error) {
	return interceptor.NewCelAuthorizer(conf.AuthorizerConfig().Rule)
}

// Setting this late in the chain so observability interceptors can monitor requests that fail authorization
const GrpcInterceptorWeight uint = 70

func NewGrpcAuthorizerServerInterceptors(a interceptor.Authorizer) (*fxgrpc.UnaryServerInterceptor, *fxgrpc.StreamServerInterceptor) {
	unaryIx := &fxgrpc.UnaryServerInterceptor{Weight: GrpcInterceptorWeight, Interceptor: interceptor.NewAuthorizerUnaryServerInterceptor(a)}
	streamIx := &fxgrpc.StreamServerInterceptor{Weight: GrpcInterceptorWeight, Interceptor: interceptor.NewAuthorizerStreamServerInterceptor(a)}
	return unaryIx, streamIx
}
