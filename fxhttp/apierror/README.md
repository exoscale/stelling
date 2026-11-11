# API errors

This package defines the errors a service returns to its clients, independently of the transport.

Every service shares a small set of error types and field error codes. A service declares its own types, as
refinements of a shared one, in its domain layer. Any layer of the service can return them, and the transport modules
convert them to their wire format: `fxhttp` writes them as [RFC 9457 Problem Details](https://www.rfc-editor.org/rfc/rfc9457),
and the `fxhttp/openapi` package documents them in the rendered specifications.

The package has no dependencies, so the domain layer does not depend on any transport.

## Shared types and codes
The shared types (`InvalidRequest`, `NotFound`, `Conflict`, ...) and the codes of the field errors (`CodeRequired`,
`CodeTooLarge`, ...) are generated from `problems.json`, a copy of the table public-api publishes in its
specification as `x-problems`. public-api holds the reference: change the table there, not here.

Do not edit `problems.json` or `problems_gen.go` by hand. The `update-problems` workflow pulls the table from the
prod specification every day, regenerates the code and opens a PR when it changed, and CI fails when the generated
code does not match `problems.json`. To regenerate by hand:

```sh
curl -fsSL https://api-ch-gva-2.exoscale.com/v2/openapi.json | jq -S '."x-problems"' > fxhttp/apierror/problems.json
go generate ./fxhttp/apierror/...
```

Use `https://ppapi-ch-gva-2.exoscale.com/v2/openapi.json` to test a table deployed in preprod only, without
committing the result.

## Declaring error types

```go
var ErrKeyDisabled = apierror.New(apierror.Type{
	Slug:        "kms/key-disabled",
	Parent:      apierror.Conflict,
	Title:       "The key is disabled.",
	Description: "The operation is not allowed on a disabled key.",
}, "")
```

* `Slug` is returned to clients as the type `#/problems/kms/key-disabled`. It is `<service>/<name>` in kebab-case,
  with the same namespace for every type of the service, and must be stable
* `Parent` is the shared type the type refines. It gives the status of its errors, and their meaning for the clients
  which do not know the service type
* `Title` is the summary of the type, the same for every occurrence. `Description` documents when it occurs

`Validate` checks these rules. The openapi package calls it on the types registered with `WithErrorTypes`, so a
service with invalid types does not start.

## Returning errors

The sentinel can be returned as is, or used to create occurrences:

```go
return ErrKeyDisabled.Type.New(fmt.Sprintf("key %q is disabled", id)) // detail is optional
return ErrKeyDisabled.Type.Wrap("", err)                               // err is the cause, never returned to clients

return apierror.InvalidRequest.WithFields(
	apierror.FieldOf(api.Fields.Coordinate, apierror.CodeRequired),
	apierror.FieldOf(api.Fields.Rules.At(i).Action, apierror.CodeUnknownValue),
)
```

Field errors name the field as the client sent it, eg `rules[0].action`, and never quote its value. Their reason is
the message of their code. A field error with an empty name is about the whole request.

Take the names from the vocabulary `apigen` generates from the specification (see the openapi package), so that a
misspelled or missing field can't compile. `F` and `FIn` remain for names which are not in a specification.

Shared types have no occurrence of their own: a service refines them with its types, or returns `InvalidRequest`
with the fields to fix. `WithFields` requires at least one, so an invalid request which names no field can't compile.
Add a detail for the client and a cause for the logs with `WithDetail` and `WithCause`:

```go
return apierror.InvalidRequest.WithFields(
	apierror.FieldOf(api.Fields.Usage, apierror.CodeNotAllowed),
	apierror.FieldOf(api.Fields.KeySpec, apierror.CodeNotAllowed),
).WithDetail("sign-verify usage is not supported with AES_256 key-spec").WithCause(err)
```

When an error is about a combination of fields, name each field of the request which takes part in it.

Errors which are not API errors are written as a 500 without detail. `InternalError(cause)` does the same while
keeping a recognizable API error at the top of the chain: clients never receive its cause, even when the cause is
itself an API error.

`errors.Is(err, ErrKeyDisabled)` matches any occurrence of the type. `From` and `TypeOf` retrieve the API error of an
error chain; errors which are not API errors are `Internal`.

Service specific members can be added to the response with `WithExtension`. Their names are in kebab-case.
