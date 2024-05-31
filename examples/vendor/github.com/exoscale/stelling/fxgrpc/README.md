# Grpc Module

This module provides [gRPC](https://pkg.go.dev/google.golang.org/grpc) support.

This package provides 2 modules:

* A grpc server module
* A grpc client module

It will also install a custom codec that uses [vtprotobuf](https://github.com/planetscale/vtprotobuf)
optimized (un)marshaling when possible.

## Server

### Components 
The server module lazily provides the following components:

* A `grpc.ServiceRegistrar`

The module adds the following features to the server:

* It will use `CertficateReloader` in case the configuration specifies TLS options.
* It will enable the [reflection service](https://pkg.go.dev/google.golang.org/grpc/reflection) on the server.
* All the interceptors from the `unary_server_interceptor` and `stream_server_interceptor` [value groups](https://uber-go.github.io/fx/value-groups/) will be installed on the server.
  Multiple stelling modules will lazily provide interceptors already.
  See the [interceptors](./interceptors) section for more details

The user needs to explicitly Invoke `StartGrpcServer` in their system. This allows fine grained control over the start and stop timing of components that do not share explicit dependencies.

The server can further be customized by providing [grpc.ServerOptions](https://pkg.go.dev/google.golang.org/grpc#ServerOption) in the `grpc_server_options` value group.

### Configuration
The module provides the following configuration options:

* `Address`: The address + port on which the grpc server will bind
* `TLS`: A boolean indicating that the server must expose using TLS
* `CertFile`: Path to the pem encoded server TLS certificate
* `KeyFile`: Path to the pem encoded private key of the server TLS certificate
* `ClientCAFile`: Path to a pem encoded CA cert bundle used to validate clients. No client validation happens if unset.

## Client

### Components 
The module lazily provides the following components:

* A `grpc.ClientConnInterface`

The module adds the following features to the client:

* It will use `CertficateReloader` in case the configuration specifies TLS options.
* All the interceptors from the `unary_client_interceptor` and `stream_client_interceptor` [value groups](https://uber-go.github.io/fx/value-groups/) will be installed on the client.
  Multiple stelling modules will lazily provide interceptors already.
  See the [interceptors](./interceptors) section for more details

### Configuration
The module provides the following configuration options:

* `InsecureConnection`: Disables TLS when connecting to the server
* `CertFile`: Path to a pem encoded client TLS certificate
* `KeyFile`: Path to the pem encoded private key of the client TLS certificate
* `RootCAFile`: Path to a pem encoded CA bundle to validate the server certificate
* `Endpoint`: The address + port (without protocol) of the grpc server

The client can further be customized by providing [grpc.DialOption](https://pkg.go.dev/google.golang.org/grpc#DialOption) in the `grpc_client_options` value group.

## Interceptors
Because grpc interceptors form a chain, the order in which they are installed on the server or client is important.

Unfortunately, fx value groups do not make any guarantees about the order in which the elements will be provided to the component.

In order have control over the place of an interceptor in the chain, this package exposes its own `Interceptor` types.
In addition to the gRPC interceptor they also include a `Weight`.

Before being installed on the gRPC server or client, the interceptors will be sorted by their `Weight` in ascending order.
Furthermore, each package in this module that provides gRPC interceptor will contain a `GrpcInterceptorWeight` constant, containing the weight assigned
to their interceptors. This allows you to place your own interceptors in the chain relative to these interceptors without having to hardcode any specific values.
