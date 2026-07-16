package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/exoscale/stelling/examples/api/public"
)

// This is the definition of a handler for a given endpoint defined in the OpenAPI spec.
// You can add new handlers in new files like this one.
func (p *APIServer) GetStellingGreeting(
	ctx context.Context, request public.GetStellingGreetingRequestObject,
) (public.GetStellingGreetingResponseObject, error) {

	if strings.Contains(p.message, "error") {
		return public.GetStellingGreeting500JSONResponse{
			Title:  "some error",
			Detail: "greeting contains the word error",
			Status: http.StatusInternalServerError,
		}, nil
	}

	return public.GetStellingGreeting200JSONResponse{
		Message: p.message,
	}, nil
}
