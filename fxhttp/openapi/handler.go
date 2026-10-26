package openapi

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/exoscale/stelling/fxhttp"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	middleware "github.com/oapi-codegen/nethttp-middleware"
)

// NewHandler returns the handler serving the given surfaces
// For each surface it:
//   - validates the requests against the specification, before they reach the handler of the surface
//   - serves the self contained specification at <BasePath>/openapi.yaml, at the SpecAliases and on the base path
//     itself, when ServeSpec is set
//   - writes request and handler errors with fxhttp.WriteError, unless overridden by an Option
func NewHandler(surfaces []*Surface, opts ...Option) (http.Handler, error) {
	o := newOptions(opts)
	catalog, err := o.catalog()
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()

	// The surface serving each pattern: the mux panics on duplicate patterns
	served := map[string]string{}
	handle := func(s *Surface, pattern string, h http.Handler) error {
		if other, ok := served[pattern]; ok {
			return fmt.Errorf("surface %q: path %q is already served by surface %q", s.Name, pattern, other)
		}
		served[pattern] = s.Name
		mux.Handle(pattern, h)
		return nil
	}

	for _, s := range surfaces {
		if err := s.validate(); err != nil {
			return nil, err
		}
		if err := catalog.validate(s.Spec.Document()); err != nil {
			return nil, fmt.Errorf("surface %q: %w", s.Name, err)
		}

		h := s.Mount(MountOptions{
			BaseURL:              s.basePath(),
			RequestErrorHandler:  o.requestErrorHandler,
			ResponseErrorHandler: o.writeResponseError,
		})
		for _, m := range slices.Backward(s.Middleware) {
			h = m(h)
		}

		h = middleware.OapiRequestValidatorWithOptions(s.Spec.Document(), &middleware.Options{
			Options:               openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
			ErrorHandlerWithOpts:  newValidatorErrorHandler(o.requestErrorHandler),
			SilenceServersWarning: true,
		})(h)
		if o.contractViolation != nil {
			if h, err = newContractCheck(s, catalog, o.contractViolation, h); err != nil {
				return nil, err
			}
		}

		mountPath := s.basePath() + "/"
		if !s.ServeSpec {
			if err := handle(s, mountPath, h); err != nil {
				return nil, err
			}
			continue
		}

		rendered, err := s.Spec.Render(append(slices.Clone(opts), s.SpecOptions...)...)
		if err != nil {
			return nil, fmt.Errorf("surface %q: %w", s.Name, err)
		}
		specHandler := newSpecHandler(s.specPath(), rendered)

		// The base path itself serves the spec, to make the API discoverable
		if err := handle(s, mountPath, newDiscoveryHandler(mountPath, specHandler, h)); err != nil {
			return nil, err
		}
		for _, p := range append([]string{s.specPath()}, s.SpecAliases...) {
			if err := handle(s, p, specHandler); err != nil {
				return nil, err
			}
		}
	}

	return mux, nil
}

// newContractCheck reports the error responses next writes which are not declared by the x-errors of their operation
func newContractCheck(s *Surface, catalog *errorCatalog, violation func(*http.Request, string, *fxhttp.Error), next http.Handler) (http.Handler, error) {
	router, err := legacy.NewRouter(s.Spec.Document())
	if err != nil {
		return nil, fmt.Errorf("surface %q: build router: %w", s.Name, err)
	}

	// The statuses of the error types declared by each operation using x-errors
	declared := map[*openapi3.Operation]map[string]int{}
	for _, item := range s.Spec.Document().Paths.Map() {
		for _, op := range item.Operations() {
			types, ok, err := catalog.declared(item, op)
			if err != nil {
				return nil, fmt.Errorf("surface %q: %w", s.Name, err)
			}
			if !ok {
				continue
			}
			statuses := make(map[string]int, len(types))
			for _, p := range types {
				statuses[p.Type] = p.Status
			}
			declared[op] = statuses
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := fxhttp.ContextWithErrorRecorder(r.Context())
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)

		e := fxhttp.RecordedError(ctx)
		if e == nil || e.Status >= http.StatusInternalServerError {
			return
		}
		route, _, err := router.FindRoute(r)
		if err != nil {
			return
		}
		statuses, ok := declared[route.Operation]
		if !ok {
			return
		}
		if status, ok := statuses[e.Type]; !ok || status != e.Status {
			violation(r, route.Operation.OperationID, e)
		}
	}), nil
}

func newValidatorErrorHandler(requestErrorHandler func(http.ResponseWriter, *http.Request, error)) middleware.ErrorHandlerWithOpts {
	return func(ctx context.Context, err error, w http.ResponseWriter, r *http.Request, opts middleware.ErrorHandlerOpts) {
		if opts.StatusCode != http.StatusBadRequest {
			fxhttp.WriteError(w, r, &fxhttp.Error{Status: opts.StatusCode, Detail: err.Error(), Cause: err})
			return
		}
		requestErrorHandler(w, r.WithContext(ctx), err)
	}
}

func newSpecHandler(specPath string, served []byte) http.Handler {
	link := "<" + specPath + ">; rel=\"service-desc\"; type=\"application/yaml\""
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Link", link)
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write(served)
	})
}

func newDiscoveryHandler(discoveryPath string, specHandler http.Handler, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == discoveryPath {
			specHandler.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
