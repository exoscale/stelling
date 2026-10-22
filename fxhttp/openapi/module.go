package openapi

import (
	"net/http"

	"github.com/exoscale/stelling/fxhttp"
	"go.uber.org/fx"
)

type SurfacesResult struct {
	fx.Out

	Surfaces []*Surface `group:"openapi_surfaces,flatten"`
}

type surfacesParams struct {
	fx.In

	Surfaces []*Surface `group:"openapi_surfaces"`
}

// NewModule serves the surfaces provided to the system through SurfacesResult
// It provides:
//   - the http.Handler of the main fxhttp server, see NewHandler
//   - an fxhttp.RPCMethodMapper resolving requests to their operation id, see OperationResolver
func NewModule(opts ...Option) fx.Option {
	return fx.Module(
		"openapi",
		fx.Supply(fx.Private, opts),
		fx.Provide(
			provideHandler,
			provideOperationResolver,
			func(r *OperationResolver) fxhttp.RPCMethodMapper { return r.Resolve },
		),
	)
}

func provideHandler(p surfacesParams, opts []Option) (http.Handler, error) {
	return NewHandler(p.Surfaces, opts...)
}

func provideOperationResolver(p surfacesParams) (*OperationResolver, error) {
	return NewOperationResolver(p.Surfaces)
}
