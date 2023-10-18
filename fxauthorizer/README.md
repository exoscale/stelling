# Authorizer Module

This module adds  [CEL](https://github.com/google/cel-spec) based middleware for authorizing gRPC and HTTP* requests.

*Soon(tm)

The authorizer allows you to write policies targetting the following parameters:

* Request headers/metadata
* TLS client cert information
* Claims in OIDC IDTokens
* Grpc service and method name
* Http uri, path and query parameters

Because the CEL program is cached and the available parameters are all readily available to request handlers,
policy evaluation introduces very little overhead.
Benchmarking shows that common policy evaluate in about 1 microsecond.

This provides a flexible environment that allows expressing a wide variety of policies.

## Components

* GrpcServerInterceptors that evaluate the given policy on each request

## Configuration file
The module supports the following configuration options:
* `Rule`: The CEL expression which will be validated for each request. Required

## Request schema
While most parameters are shared between Http and Grpc requests, they have been tailored to their respective
protocols. For the most up to date definitions check [schema/schema.proto](./schema/schema.proto).

## Example policies

* Allow healthchecks for everyone, but other requests only for a specific service (using TLS)
    ```cel
    request.service == "grpc.health.v1.Healh" || request.tls.subject.common_name == "special.client"
    ```
* Only allow clients from a specific OIDC group
    ```cel
    "dev" in request.jwt.groups
    ```

## Roadmap
* Http middleware
* Hot reloading policies
