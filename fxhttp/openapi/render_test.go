package openapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exoscale/stelling/fxhttp"
	"github.com/exoscale/stelling/fxhttp/apierror"
	"github.com/exoscale/stelling/fxhttp/openapi/openapitest"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

// errorsSpec lists its error types instead of declaring its error responses, and composes schemas with allOf
const errorsSpec = `
openapi: 3.0.0
info: {title: things, version: "1"}
servers:
  - url: /
paths:
  /things:
    get:
      operationId: list-things
      responses:
        '200':
          description: The things
          content: {application/json: {schema: {type: array, items: {$ref: '#/components/schemas/thing'}}}}
  /things/{id}:
    get:
      operationId: get-thing
      parameters:
        - {name: id, in: path, required: true, schema: {type: string, minLength: 36, maxLength: 36}}
      x-errors:
        - thing/not-found
        - type: thing/locked
          description: The thing is locked by a running migration.
      responses:
        '200':
          description: The thing
          content: {application/json: {schema: {$ref: '#/components/schemas/thing'}}}
components:
  schemas:
    named:
      type: object
      required: [name]
      properties:
        name: {type: string}
    thing:
      allOf:
        - $ref: '#/components/schemas/named'
        - type: object
          required: [id]
          properties:
            id: {type: string}
            owner: {$ref: '#/components/schemas/owner'}
    owner:
      type: object
      properties:
        email: {type: string}
`

func errorsOptions() []Option {
	return []Option{
		WithErrorTypes(ErrThingNotFound.Type, ErrThingLocked.Type),
		WithErrorExtension("owner", "owner"),
	}
}

func mustRender(t *testing.T, spec string, opts ...Option) *openapi3.T {
	t.Helper()
	data, err := mustLoadSpec(t, spec, nil).Render(opts...)
	require.NoError(t, err)
	require.NotContains(t, string(data), "allOf")
	require.NotContains(t, string(data), ErrorsExtension)

	doc, err := openapi3.NewLoader().LoadFromData(data)
	require.NoError(t, err)
	return doc
}

func TestRender(t *testing.T) {
	doc := mustRender(t, errorsSpec, errorsOptions()...)
	op := doc.Paths.Find("/things/{id}").Get

	t.Run("Should declare the error responses grouped by status", func(t *testing.T) {
		require.ElementsMatch(t, []string{"200", "400", "404", "409"}, keys(op.Responses.Map()))

		badRequest := *op.Responses.Value("400").Value.Description
		require.Contains(t, badRequest, "`"+fxhttp.TypeBadRequest+"`", "request validation is declared for operations taking input")
		require.Contains(t, *op.Responses.Value("404").Value.Description, "- `#/thing/not-found` (Thing not found): The thing does not exist.")
		require.Contains(t, *op.Responses.Value("409").Value.Description, "The thing is locked by a running migration.", "the operation description overrides the catalog one")
	})

	t.Run("Should describe the responses with the error-response schema and an example per error type", func(t *testing.T) {
		media := op.Responses.Value("404").Value.Content.Get("application/json")
		require.Equal(t, "#/components/schemas/"+errorResponseSchemaName, media.Schema.Ref)
		require.Contains(t, media.Examples, "thing/not-found")

		schema := doc.Components.Schemas[errorResponseSchemaName].Value
		require.ElementsMatch(t, []string{"type", "title", "status", "detail"}, schema.Required)
		require.Equal(t, "#/components/schemas/owner", schema.Properties["owner"].Ref)
		openapitest.AssertErrorResponses(t, doc, &fxhttp.Error{Status: http.StatusNotFound, Extensions: map[string]any{"owner": map[string]any{"email": "a@b.c"}}})
	})

	t.Run("Should flatten allOf into plain objects", func(t *testing.T) {
		thing := doc.Components.Schemas["thing"].Value
		require.Empty(t, thing.AllOf)
		require.True(t, thing.Type.Is(openapi3.TypeObject))
		require.ElementsMatch(t, []string{"name", "id", "owner"}, keys(thing.Properties))
		require.ElementsMatch(t, []string{"name", "id"}, thing.Required)
		require.Equal(t, "#/components/schemas/owner", thing.Properties["owner"].Ref, "references of the members are kept")
	})

	t.Run("Should leave operations without x-errors untouched", func(t *testing.T) {
		require.ElementsMatch(t, []string{"200"}, keys(doc.Paths.Find("/things").Get.Responses.Map()))
	})
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestRenderFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		spec string
		opts []Option
		err  string
	}{
		"unknown error type": {
			spec: errorsSpec,
			opts: []Option{WithErrorTypes(ErrThingNotFound.Type)},
			err:  `unknown error type "thing/locked"`,
		},
		"unknown error extension schema": {
			spec: errorsSpec,
			opts: append(errorsOptions(), WithErrorExtension("tenant", "tenant")),
			err:  `schema "tenant" is not a component`,
		},
		"error response declared twice": {
			spec: strings.Replace(errorsSpec, "      responses:\n        '200':\n          description: The thing\n", "      responses:\n        '404': {description: Not found}\n        '200':\n          description: The thing\n", 1),
			opts: errorsOptions(),
			err:  "declared both in responses and in x-errors",
		},
		"conflicting allOf members": {
			spec: strings.Replace(errorsSpec, "            id: {type: string}", "            id: {type: string}\n            name: {type: integer}", 1),
			opts: errorsOptions(),
			err:  `property "name" is defined differently`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := mustLoadSpec(t, tc.spec, nil).Render(tc.opts...)
			require.ErrorContains(t, err, tc.err)
		})
	}
}

func TestRenderFiles(t *testing.T) {
	target := RenderTarget{Path: filepath.Join(t.TempDir(), "api.rendered.yaml"), Spec: mustLoadSpec(t, errorsSpec, nil)}

	require.Error(t, RenderFiles([]RenderTarget{target}, true, errorsOptions()...), "a missing file is not up to date")
	require.NoError(t, RenderFiles([]RenderTarget{target}, false, errorsOptions()...))
	require.NoError(t, RenderFiles([]RenderTarget{target}, true, errorsOptions()...))

	require.NoError(t, os.WriteFile(target.Path, []byte("stale"), 0o600))
	require.ErrorContains(t, RenderFiles([]RenderTarget{target}, true, errorsOptions()...), "is not up to date")
}

var (
	ErrThingNotFound = apierror.New(apierror.Type{ID: "#/thing/not-found", Category: apierror.NotFound, Title: "Thing not found", Description: "The thing does not exist."}, "no details.")
	ErrThingLocked   = apierror.New(apierror.Type{ID: "#/thing/locked", Category: apierror.Conflict, Title: "Thing locked", Description: "The thing can not be modified right now."}, "no details.")

	errThingNotFound = errors.New("thing not found")
)

// errorsSurface returns error types depending on the requested id
func errorsSurface(t *testing.T) *Surface {
	t.Helper()
	return &Surface{
		Name:      "things",
		Spec:      mustLoadSpec(t, errorsSpec, nil),
		ServeSpec: true,
		Mount: func(o MountOptions) http.Handler {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /things/{id}", func(w http.ResponseWriter, r *http.Request) {
				switch r.PathValue("id") {
				case "00000000-0000-0000-0000-000000000404":
					o.ResponseErrorHandler(w, r, errThingNotFound)
				case "00000000-0000-0000-0000-000000000418":
					o.ResponseErrorHandler(w, r, &fxhttp.Error{Status: http.StatusTeapot, Type: "#/thing/teapot"})
				case "00000000-0000-0000-0000-000000000409":
					o.ResponseErrorHandler(w, r, fmt.Errorf("update: %w", ErrThingLocked.Type.New("migration running")))
				case "00000000-0000-0000-0000-000000000500":
					o.ResponseErrorHandler(w, r, errors.New("boom"))
				default:
					_, _ = io.WriteString(w, `{"id":"1","name":"thing"}`)
				}
			})
			return mux
		},
	}
}

func TestErrorMapperAndContractCheck(t *testing.T) {
	type violation struct {
		operationID string
		errorType   string
	}
	var violations []violation

	opts := append(errorsOptions(),
		WithErrorMapper(func(_ context.Context, err error) *fxhttp.Error {
			if errors.Is(err, errThingNotFound) {
				return &fxhttp.Error{Status: http.StatusNotFound, Type: "#/thing/not-found", Title: "Thing not found", Detail: err.Error()}
			}
			return nil
		}),
		WithContractViolationHandler(func(_ *http.Request, operationID string, e *fxhttp.Error) {
			violations = append(violations, violation{operationID, e.Type})
		}),
	)
	h, err := NewHandler([]*Surface{errorsSurface(t)}, opts...)
	require.NoError(t, err)

	t.Run("Should map handler errors to their error type", func(t *testing.T) {
		violations = nil
		e := requireError(t, serve(t, h, http.MethodGet, "/things/00000000-0000-0000-0000-000000000404", ""), http.StatusNotFound)
		require.Equal(t, "#/thing/not-found", e.Type)
		require.Empty(t, violations)
	})

	t.Run("Should allow request validation failures and server errors", func(t *testing.T) {
		violations = nil
		_ = requireError(t, serve(t, h, http.MethodGet, "/things/short", ""), http.StatusBadRequest)
		_ = requireError(t, serve(t, h, http.MethodGet, "/things/00000000-0000-0000-0000-000000000500", ""), http.StatusInternalServerError)
		require.Empty(t, violations)
	})

	t.Run("Should write API errors with the status of their category", func(t *testing.T) {
		violations = nil
		e := requireError(t, serve(t, h, http.MethodGet, "/things/00000000-0000-0000-0000-000000000409", ""), http.StatusConflict)
		require.Equal(t, "#/thing/locked", e.Type)
		require.Equal(t, "migration running", e.Detail)
		require.Empty(t, violations)
	})

	t.Run("Should report undeclared error types", func(t *testing.T) {
		violations = nil
		_ = requireError(t, serve(t, h, http.MethodGet, "/things/00000000-0000-0000-0000-000000000418", ""), http.StatusTeapot)
		require.Equal(t, []violation{{"get-thing", "#/thing/teapot"}}, violations)
	})

	t.Run("Should serve the rendered spec", func(t *testing.T) {
		rec := serve(t, h, http.MethodGet, "/openapi.yaml", "")
		require.Equal(t, http.StatusOK, rec.Code)
		rendered, err := mustLoadSpec(t, errorsSpec, nil).Render(opts...)
		require.NoError(t, err)
		require.Equal(t, string(rendered), rec.Body.String())
	})

	t.Run("Should fail on unknown error types of surfaces which do not serve their spec", func(t *testing.T) {
		s := errorsSurface(t)
		s.ServeSpec = false
		_, err := NewHandler([]*Surface{s})
		require.ErrorContains(t, err, "unknown error type")
	})
}

func TestSurfaceSpecOptions(t *testing.T) {
	errorTypes := errorsOptions()[0]
	s := errorsSurface(t)
	s.SpecOptions = []Option{WithErrorExtension("owner", "owner")}

	h, err := NewHandler([]*Surface{s}, errorTypes)
	require.NoError(t, err)
	rec := serve(t, h, http.MethodGet, "/openapi.yaml", "")
	require.Equal(t, http.StatusOK, rec.Code)

	doc, err := openapi3.NewLoader().LoadFromData(rec.Body.Bytes())
	require.NoError(t, err)
	require.Contains(t, doc.Components.Schemas[errorResponseSchemaName].Value.Properties, "owner", "the options of the surface apply to its served spec")

	target := RenderTarget{Path: filepath.Join(t.TempDir(), "api.rendered.yaml"), Spec: s.Spec, Options: s.SpecOptions}
	require.NoError(t, RenderFiles([]RenderTarget{target}, false, errorTypes))
	rendered, err := os.ReadFile(target.Path)
	require.NoError(t, err)
	require.Equal(t, rec.Body.String(), string(rendered), "the rendered file is the served spec")
}

func TestStatusMapping(t *testing.T) {
	// A service returning every client error as a 400
	allBadRequest := WithStatusMapping(func(c apierror.Category) int {
		if c.IsClient() {
			return http.StatusBadRequest
		}
		return http.StatusInternalServerError
	})
	opts := append(errorsOptions(), allBadRequest)

	h, err := NewHandler([]*Surface{errorsSurface(t)}, opts...)
	require.NoError(t, err)
	_ = requireError(t, serve(t, h, http.MethodGet, "/things/00000000-0000-0000-0000-000000000409", ""), http.StatusBadRequest)

	doc := mustRender(t, errorsSpec, opts...)
	responses := doc.Paths.Find("/things/{id}").Get.Responses
	require.ElementsMatch(t, []string{"200", "400"}, keys(responses.Map()), "the rendered spec uses the same statuses")
	require.Contains(t, *responses.Value("400").Value.Description, "#/thing/locked")
}
