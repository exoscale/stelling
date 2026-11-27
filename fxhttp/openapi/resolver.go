package openapi

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
)

// unmatchedOperationID is the operation id of requests that do not match any operation
const unmatchedOperationID = "unmatched"

type resolvedSurface struct {
	surface *Surface
	router  routers.Router
}

// OperationResolver maps requests to the id of the OpenAPI operation they call
// Use its Resolve method as an fxhttp.RPCMethodMapper, to label the observability data with it
// Unlike the url path, the operation id has a bounded cardinality
type OperationResolver struct {
	surfaces  []resolvedSurface
	specPaths map[string]string
}

func NewOperationResolver(surfaces []*Surface) (*OperationResolver, error) {
	r := &OperationResolver{specPaths: map[string]string{}}
	for _, s := range surfaces {
		if err := s.validate(); err != nil {
			return nil, err
		}
		router, err := legacy.NewRouter(s.Spec.Document())
		if err != nil {
			return nil, fmt.Errorf("surface %q: build router: %w", s.Name, err)
		}
		r.surfaces = append(r.surfaces, resolvedSurface{surface: s, router: router})
		if s.ServeSpec {
			r.specPaths[s.specPath()] = s.specOperationID()
			for _, alias := range s.SpecAliases {
				r.specPaths[alias] = s.specOperationID()
			}
		}
	}
	// The most specific base path wins
	slices.SortFunc(r.surfaces, func(a, b resolvedSurface) int {
		return len(b.surface.basePath()) - len(a.surface.basePath())
	})
	return r, nil
}

// Resolve returns the operation id of req, or unmatchedOperationID
func (r *OperationResolver) Resolve(req *http.Request) string {
	if req == nil || req.URL == nil {
		return unmatchedOperationID
	}
	if opID, ok := r.specPaths[req.URL.Path]; ok {
		return opID
	}

	for _, s := range r.surfaces {
		if base := s.surface.basePath(); base != "" && !strings.HasPrefix(req.URL.Path, base+"/") {
			continue
		}
		route, _, err := s.router.FindRoute(req)
		if err != nil || route == nil || route.Operation == nil || route.Operation.OperationID == "" {
			return unmatchedOperationID
		}
		return route.Operation.OperationID
	}

	return unmatchedOperationID
}
