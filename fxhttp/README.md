# HTTP Module

This module provides [http server](https://pkg.go.dev/net/http) support.

This package provides 2 modules:

* A regular module which provides a top level http server
  The other stelling modules will add various middleware to this server
* A "named" module, which provides a named submodule: this one is intended to be used in other stelling packages, such as metrics.
  It doesn't provide all the features of the main module. To create the main http server of your application you should use the regular module.

## Components 
The module lazily provides the following components:

* A `*http.Server`
* A `*fxhttp.DrainState`, which reports whether the process is shutting down

You must supply a single `http.Handler` in the system, which will be set as the servers handler.
This way, the module does not enforce any opinions on how the server mux is constructed.

The client `Invoke`s `StartHttpServer` explicitly in his system.
This allows fine grained control over the start and stop order of components that do not share a dependency.

The module will also use `CertficateReloader` in case the configuration specifies TLS options.

When the server stops, it enters draining before it stops accepting connections. Use the `*fxhttp.DrainState`
to fail readiness checks or stop admitting new work. While draining, HTTP/1.x responses carry `Connection: close`
so clients reconnect to another instance.

## Options
* WithRPCMethodMapper
  Passes in a function which returns the name of the rpc method a particular request is calling.
  This information will be added as metadata to the observability
* WithServerModuleName
  This option allows creating additional "named" http servers. It is intended to be used by other stelling
  modules, such as the metrics. Named servers do not provide all the features of the main server: for example
  no middleware will be installed.
* WithRequestID
  Ensures every request carries an `x-request-id`: the incoming one is kept, or a new one is generated.
  The id is available through `RequestIDFromContext`, and logged by the logging module.

## Exoscale IAM identity
Services behind the Exoscale public API receive the id of the authenticated IAM identity in the
`x-exoscale-iam-identity-id` header. `IAMIdentityHandler` stores it in the request context, where
`IAMIdentityIDFromContext` retrieves it. Only install it on routes that are exclusively reachable through
the public API, as the header is trusted as is.

## Errors
The package defines a standard shape for error responses: `fxhttp.Error`, which follows
[RFC 9457 Problem Details](https://www.rfc-editor.org/rfc/rfc9457):

```json
{"type": "#/http-problem-types/request-invalid-parameters", "title": "Bad Request", "status": 400, "detail": "invalid id"}
```

Service specific members can be added through `Extensions`. They are serialized as top level
members and can not override the standard ones. The `Cause` is never serialized.

`WriteError(w, r, err)` writes any error in this shape, with `Content-Type: application/json`:

* If `err` wraps an `*fxhttp.Error`, it is written as is. Missing fields are defaulted from the status.
* If `err` wraps a `*apierror.Error`, see the [apierror](apierror) package, it is written with the `DefaultStatus`
  of its category.
* Any other error is written as a 500 with the error message as `detail`.

The signature of `WriteError` matches the error hooks of common routers and code generators, so it
can be passed to them directly. For example with [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen):

```go
strictHandler := api.NewStrictHandlerWithOptions(server, nil, api.StrictHTTPServerOptions{
    RequestErrorHandlerFunc:  fxhttp.WriteError,
    ResponseErrorHandlerFunc: fxhttp.WriteError,
})
handler := api.HandlerWithOptions(strictHandler, api.StdHTTPServerOptions{
    ErrorHandlerFunc: fxhttp.WriteError,
})
```

Return an `*fxhttp.Error` from your handler to control the status and content of the response.

`WriteError` records the error on the request: the logging module adds it to the request log line, and the
tracing module records server errors on the request span.

The shape is defined in Go only: if your API is described by a specification, it should describe
this shape itself. See the [openapi](./openapi) package to serve APIs described by OpenAPI specifications. `fxhttp.Error` implements `json.Unmarshaler`, which makes it easy to write a test
asserting the two stay in sync, and to parse error responses in clients.

## Configuration
The module provides the following configuration options:
* `Address`: The address + port, or a unix filepath, on which the http server will bind
* `TLS`: A boolean indicating that the server must expose using TLS
* `CertFile`: Path to the pem encoded server TLS certificate
* `KeyFile`: Path to the pem encoded private key of the server TLS certificate
* `ClientCAFile`: Path to a pem encoded CA cert bundle used to validate clients. No client validation happens if unset.

## Client transport
Services build their http clients with `NewTransport`, which is based on `http.DefaultTransport` and is also able
to reach servers listening on a unix domain socket, through urls built with `UnixSocketURL`. Redirects to a socket
are refused.

`WrapTransport` installs the middleware of the `http_client_middleware` value group on the transport: the tracing
and metrics modules lazily provide middleware that trace and count all requests. Inject them with
`fx.ParamTags(`group:"http_client_middleware"`)`.

`ErrorFromResponse` parses the error response of a server into an `*fxhttp.Error`.
