# OpenAPI Module

This module serves HTTP APIs described by [OpenAPI](https://spec.openapis.org/oas/) specifications, in an
API-first fashion: the specification is written first, the server code is generated from it with
[oapi-codegen](https://github.com/oapi-codegen/oapi-codegen), and every request is validated against it.

The module does not abstract over the generated code: you mount the generated handlers yourself, and the module
provides the plumbing around them.

## Surfaces
A service can expose several APIs, eg a public one and an operator one. Each of them is a `Surface`: an API
described by a single specification and mounted under a base path.

For each surface the module:

* Validates the requests against the specification before they reach the handlers.
  Invalid requests are written as a 400 `fxhttp.Error`
* Writes the errors returned by the handlers with `fxhttp.WriteError`
* Serves the specification at `<BasePath>/openapi.yaml`, at the `SpecAliases`, and on the base path itself
  to make the API discoverable, when `ServeSpec` is set. Only set it for surfaces whose specification may be
  disclosed to their clients. The served specification is self contained: components referenced from other
  files are inlined under the name they have in their own file, so consumers that only understand local
  references can use it
* Declares the error responses of the operations from the error types they list, see Error responses
* Resolves requests to the id of the operation they call, used as the `rpc.method` of the observability data.
  Unlike the url path, which contains identifiers, it has a bounded cardinality

```go
//go:embed api/public/api.yaml
var publicSpec []byte

func ProvideSurfaces(server *Server) (openapi.SurfacesResult, error) {
	spec, err := openapi.LoadSpec(publicSpec, nil)
	if err != nil {
		return openapi.SurfacesResult{}, err
	}
	return openapi.SurfacesResult{Surfaces: []*openapi.Surface{{
		Name:      "public",
		BasePath:  "/",
		Spec:      spec,
		ServeSpec: true,
		Mount: func(o openapi.MountOptions) http.Handler {
			strict := public.NewStrictHandlerWithOptions(server, nil, public.StrictHTTPServerOptions{
				RequestErrorHandlerFunc:  o.RequestErrorHandler,
				ResponseErrorHandlerFunc: o.ResponseErrorHandler,
			})
			return public.HandlerWithOptions(strict, public.StdHTTPServerOptions{
				BaseRouter:       http.NewServeMux(),
				BaseURL:          o.BaseURL,
				ErrorHandlerFunc: o.RequestErrorHandler,
			})
		},
	}}}, nil
}

func main() {
	fx.New(
		fxhttp.NewModule(conf, fxhttp.WithRequestID()),
		openapi.NewModule(),
		fx.Provide(NewServer, ProvideSurfaces),
		fx.Invoke(fxhttp.StartHttpServer),
	).Run()
}
```

`NewHandler` and `NewOperationResolver` provide the same features outside of an fx system.

## Components
The module lazily provides the following components:

* The `http.Handler` of the main `fxhttp` server
* An `fxhttp.RPCMethodMapper`
* An `*openapi.OperationResolver`

The surfaces are collected from the `openapi_surfaces` value group, see `SurfacesResult`.

## Options
* WithRequestErrorHandler
  Replaces the handler writing the response of requests that fail validation, decoding or binding
* WithErrorMapper
  Converts the errors returned by handlers into their `fxhttp.Error`, eg to log them. It is optional for API errors,
  see the [apierror](../apierror) package, which are converted with the status of their category
* WithStatusMapping
  Sets the http status of the API errors of each category, both in the responses and in the rendered specification.
  Defaults to `fxhttp.DefaultStatus`
* WithErrorTypes and WithErrorExtension
  Document the error types and the service specific members of the error responses, see below
* WithContractViolationHandler
  Called when an operation writes an error response it does not declare, see below

## Error responses
The services do not describe their error responses: they list the error types each operation can return,
and stelling declares the responses in the rendered specification.

The error types are declared once, with the [apierror](../apierror) package, typically in the domain layer
of the service. Register them, so they can be listed by the operations:

```go
var ErrThingNotFound = apierror.New(apierror.Type{ID: "#/thing/not-found", Category: apierror.NotFound,
	Title: "Thing not found", Description: "The thing does not exist."}, "no details.")

openapi.WithErrorTypes(ErrThingNotFound.Type)
```

Each operation lists the ids of its error types, without the leading `#/`, in `x-errors`. An entry can override
the description for the operation:

```yaml
    get:
      operationId: get-thing
      x-errors:
        - thing/not-found
        - type: thing/locked
          description: The thing is locked by a running migration.
```

Request validation failures (`#/http-problem-types/request-invalid-parameters`) are declared automatically for
every operation taking input. Register an error type with the same ID to customize their documentation.

Handlers return their API errors as is, which are written with the status of their category.

Operations without `x-errors` are left untouched: they can still declare their error responses themselves.

### Contract
Unknown error types fail at startup. With `WithContractViolationHandler`, every error response with an error
type the operation does not declare, or with another status, is reported: fail your tests with it, and log it in
production. Request validation failures and server errors (5xx) are always allowed.

## Rendered specification
`Spec.Render` returns the specification as its clients consume it:

* External references are inlined, under the name the components have in their own file
* The error responses of the operations using `x-errors` are declared, grouped by status. Their description lists
  the error types, with an example for each of them. Their schema is `error-response`, generated from `fxhttp.Error`
  and the extensions registered with `WithErrorExtension`. It replaces any `error-response` schema of the specification
* `allOf` compositions of object schemas are flattened: they may be used for convenience, but do not appear in the
  rendered specification. Compositions which can not be flattened without changing their meaning fail
* `x-errors` is removed

The result is validated, examples included. It is the specification served at `<BasePath>/openapi.yaml`.

Stelling does not hold any specification file: services render theirs with a small command, and commit the result
for the consumers which read it from the repository:

```go
// cmd/openapi-render/main.go, invoked with //go:generate go run ../../cmd/openapi-render
func main() {
	openapi.RenderMain([]openapi.RenderTarget{{Path: "public/api.rendered.yaml", Spec: publicSpec}}, specOptions...)
}
```

`RenderMain` takes a `-check` flag to verify the files are up to date instead of writing them.

Options which only apply to one specification, such as an error extension only some surfaces return, are set
in `Surface.SpecOptions` for the served specification and `RenderTarget.Options` for the rendered file.

## Specification conventions
* Every operation has an `operationId`
* Every operation taking parameters or a request body lists its error types in `x-errors`, or declares a 400
  response with an `error-response` schema describing `fxhttp.Error`
* 5xx responses are not part of the contract, but are written in the same shape
* `servers` declares the base path of the surface, eg `- url: /internal`

`openapitest.AssertErrorResponses` asserts a rendered specification follows these conventions, and that
`fxhttp.Error` validates against its `error-response` schema:

```go
func TestSpec(t *testing.T) {
	spec, err := openapi.LoadSpec(publicSpec, nil)
	require.NoError(t, err)
	data, err := spec.Render(specOptions...)
	require.NoError(t, err)
	doc, err := openapi3.NewLoader().LoadFromData(data)
	require.NoError(t, err)
	openapitest.AssertErrorResponses(t, doc)
}
```

## Code generation
Each surface lives in its own package, next to its specification, a `cfg.yaml` and a `gen.go`:

```yaml
# cfg.yaml
package: public
output: api.gen.go
generate:
  models: true
  std-http-server: true
  strict-server: true
# Specifications referencing components of another one map it to its package
import-mapping:
  ../public/api.yaml: github.com/exoscale/my-service/api/public
```

```go
// gen.go
package public

//go:generate go tool oapi-codegen -config cfg.yaml api.yaml
```

Pin oapi-codegen with `go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen`, commit the
generated code, and verify it is up to date in CI, eg with Earthly:

```earthfile
check-openapi-generated:
    FROM +source
    RUN cp -a api /tmp/openapi-current
    RUN go generate ./api/...
    RUN diff -ruN /tmp/openapi-current api || { echo "Generated OpenAPI bindings are out of date." >&2; exit 1; }
```

The strict server only lets handlers return the responses declared in the specification. Return the 4xx
responses of the specification as typed responses, and an error for anything unexpected: it is written as a 500.
