package openapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/exoscale/stelling/fxhttp"
	"github.com/exoscale/stelling/fxhttp/apierror"
	"github.com/exoscale/stelling/fxhttp/openapi/openapitest"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

const publicSpec = `
openapi: 3.0.0
info: {title: public, version: "1"}
servers:
  - url: /
paths:
  /things/{id}:
    get:
      operationId: get-thing
      parameters:
        - {name: id, in: path, required: true, schema: {type: string, format: uuid, minLength: 36, maxLength: 36}}
      responses:
        '200':
          description: The thing
          content: {application/json: {schema: {type: object}}}
        '400':
          description: "Invalid request: #/problems/invalid-request"
          content: {application/problem+json: {schema: {$ref: '#/components/schemas/Problem'}}}
components:
  schemas:
    Problem:
      type: object
      required: [type, title, status]
      properties:
        type: {type: string, format: uri-reference}
        title: {type: string}
        status: {type: integer, minimum: 100, maximum: 599}
        detail: {type: string}
`

const internalSpec = `
openapi: 3.0.0
info: {title: internal, version: "1"}
servers:
  - url: /internal
paths:
  /health:
    get:
      operationId: get-health
      responses:
        '200': {description: Healthy}
  /replicas:
    put:
      operationId: put-replica
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [zone]
              properties: {zone: {type: string}}
      responses:
        '200': {description: Stored}
        '400':
          description: Invalid request
          content: {application/problem+json: {schema: {$ref: '../public/api.yaml#/components/schemas/Problem'}}}
`

func mustLoadSpec(t *testing.T, data string, refs map[string][]byte) *Spec {
	t.Helper()
	s, err := LoadSpec([]byte(data), refs)
	require.NoError(t, err)
	return s
}

func publicRefs() map[string][]byte {
	return map[string][]byte{"public/api.yaml": []byte(publicSpec)}
}

// testSurfaces mounts hand written handlers in place of generated code
func testSurfaces(t *testing.T) []*Surface {
	t.Helper()

	public := &Surface{
		Name:        "public",
		BasePath:    "/",
		Spec:        mustLoadSpec(t, publicSpec, nil),
		ServeSpec:   true,
		SpecAliases: []string{"/api.yaml"},
		Mount: func(o MountOptions) http.Handler {
			mux := http.NewServeMux()
			mux.HandleFunc("GET "+o.BaseURL+"/things/{id}", func(w http.ResponseWriter, r *http.Request) {
				if r.PathValue("id") == "00000000-0000-0000-0000-000000000000" {
					o.ResponseErrorHandler(w, r, &fxhttp.Error{Status: http.StatusInternalServerError, Detail: "store unavailable"})
					return
				}
				_, _ = io.WriteString(w, `{}`)
			})
			return mux
		},
		Middleware: []func(http.Handler) http.Handler{fxhttp.IAMIdentityHandler},
	}
	internal := &Surface{
		Name:     "internal",
		BasePath: "/internal",
		Spec:     mustLoadSpec(t, internalSpec, publicRefs()),
		Mount: func(o MountOptions) http.Handler {
			mux := http.NewServeMux()
			mux.HandleFunc("GET "+o.BaseURL+"/health", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			mux.HandleFunc("PUT "+o.BaseURL+"/replicas", func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					o.RequestErrorHandler(w, r, err)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})
			return mux
		},
	}
	return []*Surface{public, internal}
}

func serve(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func requireError(t *testing.T, rec *httptest.ResponseRecorder, status int) *fxhttp.Error {
	t.Helper()
	require.Equal(t, status, rec.Code, rec.Body.String())
	require.Equal(t, fxhttp.ContentType, rec.Header().Get("Content-Type"))
	e := &fxhttp.Error{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), e))
	require.Equal(t, status, e.Status)
	return e
}

func TestSpec(t *testing.T) {
	t.Run("Should resolve external refs from the given files", func(t *testing.T) {
		s := mustLoadSpec(t, internalSpec, publicRefs())
		op := s.Document().Paths.Find("/replicas").Put
		require.NotNil(t, openapitest.ErrorResponseSchema(op).Value)
	})

	t.Run("Should fail on unknown external refs", func(t *testing.T) {
		_, err := LoadSpec([]byte(internalSpec), map[string][]byte{"other.yaml": []byte(publicSpec)})
		require.Error(t, err)
	})

	t.Run("Should render a self contained spec with the component names of their own document", func(t *testing.T) {
		data, err := mustLoadSpec(t, internalSpec, publicRefs()).Render()
		require.NoError(t, err)
		require.Contains(t, string(data), "$ref: '#/components/schemas/Problem'")
		require.NotContains(t, string(data), "public/api.yaml")

		doc, err := openapi3.NewLoader().LoadFromData(data)
		require.NoError(t, err)
		require.Contains(t, doc.Components.Schemas, "Problem")
		require.NoError(t, doc.Validate(context.Background()))
	})

	t.Run("Should fail when inlined components collide", func(t *testing.T) {
		colliding := internalSpec + `
components:
  schemas:
    Problem: {type: string}
`
		_, err := mustLoadSpec(t, colliding, publicRefs()).Render()
		require.ErrorContains(t, err, "collides")
	})
}

func TestNewHandler(t *testing.T) {
	h, err := NewHandler(testSurfaces(t))
	require.NoError(t, err)

	t.Run("Should route requests to their surface", func(t *testing.T) {
		require.Equal(t, http.StatusOK, serve(t, h, http.MethodGet, "/things/2b0f2c39-7a30-4a7a-9b3b-6c2f1f0d6e21", "").Code)
		require.Equal(t, http.StatusOK, serve(t, h, http.MethodGet, "/internal/health", "").Code)
		require.Equal(t, http.StatusNoContent, serve(t, h, http.MethodPut, "/internal/replicas", `{"zone":"ch-gva-2"}`).Code)
	})

	t.Run("Should write validation failures as 400 error responses", func(t *testing.T) {
		e := requireError(t, serve(t, h, http.MethodGet, "/things/not-a-uuid", ""), http.StatusBadRequest)
		require.Equal(t, apierror.InvalidRequest.ID(), e.Type)
		require.Equal(t, []apierror.Field{apierror.FIn("path", "id", apierror.CodeTooSmall)}, e.Errors)
		require.Empty(t, e.Detail, "the validation message can quote the value")

		e = requireError(t, serve(t, h, http.MethodPut, "/internal/replicas", `{}`), http.StatusBadRequest)
		require.Equal(t, []apierror.Field{apierror.F("zone", apierror.CodeRequired)}, e.Errors)
	})

	t.Run("Should write handler errors as error responses", func(t *testing.T) {
		e := requireError(t, serve(t, h, http.MethodGet, "/things/00000000-0000-0000-0000-000000000000", ""), http.StatusInternalServerError)
		require.Equal(t, "store unavailable", e.Detail)
	})

	t.Run("Should write unknown routes as error responses", func(t *testing.T) {
		_ = requireError(t, serve(t, h, http.MethodGet, "/unknown", ""), http.StatusNotFound)
	})

	t.Run("Should only serve the specs of surfaces with ServeSpec", func(t *testing.T) {
		_ = requireError(t, serve(t, h, http.MethodGet, "/internal/openapi.yaml", ""), http.StatusNotFound)
		_ = requireError(t, serve(t, h, http.MethodGet, "/internal/", ""), http.StatusNotFound)
	})

	t.Run("Should serve the self contained specs", func(t *testing.T) {
		surfaces := testSurfaces(t)
		surfaces[1].ServeSpec = true
		h, err := NewHandler(surfaces)
		require.NoError(t, err)

		for target, specPath := range map[string]string{
			"/":                      "/openapi.yaml",
			"/openapi.yaml":          "/openapi.yaml",
			"/api.yaml":              "/openapi.yaml",
			"/internal/":             "/internal/openapi.yaml",
			"/internal/openapi.yaml": "/internal/openapi.yaml",
		} {
			rec := serve(t, h, http.MethodGet, target, "")
			require.Equal(t, http.StatusOK, rec.Code, target)
			require.Equal(t, "application/yaml; charset=utf-8", rec.Header().Get("Content-Type"), target)
			require.Equal(t, "<"+specPath+">; rel=\"service-desc\"; type=\"application/yaml\"", rec.Header().Get("Link"), target)
			_, err := openapi3.NewLoader().LoadFromData(rec.Body.Bytes())
			require.NoError(t, err, target)
		}
	})

	t.Run("Should apply the surface middleware", func(t *testing.T) {
		var identityID string
		surfaces := testSurfaces(t)
		surfaces[0].Middleware = append(surfaces[0].Middleware, func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				identityID = fxhttp.IAMIdentityIDFromContext(r.Context())
				next.ServeHTTP(w, r)
			})
		})
		h, err := NewHandler(surfaces)
		require.NoError(t, err)

		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/things/2b0f2c39-7a30-4a7a-9b3b-6c2f1f0d6e21", nil)
		req.Header.Set(fxhttp.IAMIdentityIDHeader, "identity-1")
		h.ServeHTTP(httptest.NewRecorder(), req)
		require.Equal(t, "identity-1", identityID)
	})

	t.Run("Should apply the surface middleware after request validation", func(t *testing.T) {
		called := false
		surfaces := testSurfaces(t)
		surfaces[0].Middleware = []func(http.Handler) http.Handler{func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				next.ServeHTTP(w, r)
			})
		}}
		h, err := NewHandler(surfaces)
		require.NoError(t, err)

		_ = requireError(t, serve(t, h, http.MethodGet, "/things/not-a-uuid", ""), http.StatusBadRequest)
		require.False(t, called)
	})

	t.Run("Should not check the security requirements", func(t *testing.T) {
		secured := strings.Replace(internalSpec, "      operationId: get-health\n", "      operationId: get-health\n      security: [{apiKey: []}]\n", 1) + `
components:
  securitySchemes:
    apiKey: {type: apiKey, in: header, name: X-Api-Key}
`
		surfaces := testSurfaces(t)
		surfaces[1].Spec = mustLoadSpec(t, secured, publicRefs())

		h, err := NewHandler(surfaces)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, serve(t, h, http.MethodGet, "/internal/health", "").Code)
	})

	t.Run("Should use the given request error handler", func(t *testing.T) {
		h, err := NewHandler(testSurfaces(t), WithRequestErrorHandler(func(w http.ResponseWriter, r *http.Request, err error) {
			fxhttp.WriteError(w, r, &fxhttp.Error{Status: http.StatusBadRequest, Title: "Invalid Request", Detail: err.Error()})
		}))
		require.NoError(t, err)
		e := requireError(t, serve(t, h, http.MethodGet, "/things/not-a-uuid", ""), http.StatusBadRequest)
		require.Equal(t, "Invalid Request", e.Title)
	})

	t.Run("Should reject invalid surfaces", func(t *testing.T) {
		_, err := NewHandler([]*Surface{{Name: "broken"}})
		require.Error(t, err)

		surfaces := testSurfaces(t)
		surfaces[1].SpecAliases = []string{"/internal.yaml"}
		_, err = NewHandler(surfaces)
		require.ErrorContains(t, err, "require ServeSpec")
	})

	t.Run("Should reject base paths not declared by the servers of the spec", func(t *testing.T) {
		surfaces := testSurfaces(t)
		surfaces[1].BasePath = "/private"
		_, err := NewHandler(surfaces)
		require.ErrorContains(t, err, `base path "/private" is not declared by the servers of the spec`)

		surfaces = testSurfaces(t)
		surfaces[1].Spec = mustLoadSpec(t, strings.Replace(internalSpec, "servers:\n  - url: /internal\n", "", 1), publicRefs())
		_, err = NewHandler(surfaces)
		require.ErrorContains(t, err, "must declare the base path")
	})

	t.Run("Should reject surfaces serving the same path", func(t *testing.T) {
		surfaces := testSurfaces(t)
		surfaces[1].ServeSpec = true
		surfaces[1].SpecAliases = []string{"/api.yaml"}
		_, err := NewHandler(surfaces)
		require.ErrorContains(t, err, `path "/api.yaml" is already served by surface "public"`)
	})
}

func TestOperationResolver(t *testing.T) {
	r, err := NewOperationResolver(testSurfaces(t))
	require.NoError(t, err)

	for target, opID := range map[string]string{
		"/things/2b0f2c39-7a30-4a7a-9b3b-6c2f1f0d6e21": "get-thing",
		"/internal/health":       "get-health",
		"/openapi.yaml":          "get-public-openapi-spec",
		"/api.yaml":              "get-public-openapi-spec",
		"/internal/openapi.yaml": unmatchedOperationID,
		"/internal/unknown":      unmatchedOperationID,
		"/unknown":               unmatchedOperationID,
	} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
		require.Equal(t, opID, r.Resolve(req), target)
	}
	require.Equal(t, unmatchedOperationID, r.Resolve(nil))
}

func TestModule(t *testing.T) {
	var h http.Handler
	var mapper fxhttp.RPCMethodMapper

	app := fxtest.New(
		t,
		NewModule(),
		fx.Provide(func() SurfacesResult { return SurfacesResult{Surfaces: testSurfaces(t)} }),
		fx.Populate(&h, &mapper),
	)
	app.RequireStart()
	t.Cleanup(app.RequireStop)

	require.Equal(t, http.StatusOK, serve(t, h, http.MethodGet, "/internal/health", "").Code)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/internal/health", nil)
	require.Equal(t, "get-health", mapper(req))
}

func TestAssertErrorResponses(t *testing.T) {
	openapitest.AssertErrorResponses(t, mustLoadSpec(t, publicSpec, nil).Document())
	openapitest.AssertErrorResponses(t, mustLoadSpec(t, internalSpec, publicRefs()).Document(), &fxhttp.Error{Status: http.StatusBadRequest, Extensions: map[string]any{"zone": "ch-gva-2"}})
}
