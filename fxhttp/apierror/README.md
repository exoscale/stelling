# API errors

This package defines the errors a service returns to its clients, independently of the transport.

A service declares each kind of error once, as a sentinel, in its domain layer. Any layer of the service can
return it, and the transport modules convert it to their wire format: `fxhttp` writes it as an
[RFC 9457 Problem Details](https://www.rfc-editor.org/rfc/rfc9457) response, and the `fxhttp/openapi` package
documents it in the rendered specifications.

The package has no dependencies, so the domain layer does not depend on any transport.

## Declaring error types

```go
var ErrKeyNotFound = apierror.New(apierror.Type{
	ID:          "#/kms-key/not-found",
	Category:    apierror.NotFound,
	Title:       "Key not found",
	Description: "The request was rejected because the specified KMS Key could not be found.",
}, "no details.")
```

* `ID` is the type returned to clients. Keep it specific to the resource and the failure, eg `#/key-store/not-found`
  rather than a generic not found: clients rely on it to handle the error, and it must be stable
* `Category` classifies the kind of failure, like a gRPC status code. Transports derive their status from it
* `Title` is the short summary of the error, and `Description` documents when it occurs

## Returning errors

The sentinel can be returned as is, or used to create occurrences with a specific detail and cause:

```go
return ErrKeyNotFound.Type.New(fmt.Sprintf("key %q not found", id))
return ErrKeyNotFound.Type.Wrap("lookup failed", err)
```

`errors.Is(err, ErrKeyNotFound)` matches any occurrence of the type. `From`, `TypeOf` and `CategoryOf` retrieve
the API error of an error chain.

Service specific members can be added to the response with `WithExtension`. The cause is never returned to clients.

## Status codes
`fxhttp.DefaultStatus` maps the categories to http statuses (400, 404, 409, and 500 for internal errors). A service with another policy,
eg returning every client error as a 400, sets it with `openapi.WithStatusMapping`: it applies both to the error
responses and to their documentation.
