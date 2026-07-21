package api

import (
	"context"

	"github.com/exoscale/stelling/examples/api/internal"
)

func (p *APIServer) Healthz(
	ctx context.Context,
	request internal.HealthzRequestObject,
) (internal.HealthzResponseObject, error) {
	return internal.Healthz200JSONResponse{Message: "OK"}, nil
}
