# Grpc Module

This module provides [gRPC](https://pkg.go.dev/google.golang.org/grpc) support.

This package provides 2 modules:

* A grpc server module
* A grpc client module

## Server

### Components 
The server module lazily provides the following components:

* A `*grpc.Server`

The module adds the following features to the server:

* It will use `CertficateReloader` in case the configuration specifies TLS options.
* All the middleware from the `unary_server_interceptor` and `stream_server_interceptor` [value groups](https://uber-go.github.io/fx/value-groups/) will be installed on the server.
  Multiple stelling modules will lazily provide middleware already.

The user needs to explicitly Invoke `StartGrpcServer` in their system. This allows fine grained control over the start and stop timing of components that do not share explicit dependencies.

TODO: The server can further be customized by providing [grpc.ServerOptions](https://pkg.go.dev/google.golang.org/grpc#ServerOption) in the `grpc_server_options` value group.

### Configuration
The module provides the following configuration options:
* `Port`: The port on which the grpc server will bind
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
* All the middleware from the `unary_client_interceptor` and `stream_client_interceptor` [value groups](https://uber-go.github.io/fx/value-groups/) will be installed on the client.
  Multiple stelling modules will lazily provide middleware already.

### Configuration
The module provides the following configuration options:
* `InsecureConnection`: Disables TLS when connecting to the server
* `CertFile`: Path to a pem encoded client TLS certificate
* `KeyFile`: Path to the pem encoded private key of the client TLS certificate
* `RootCAFile`: Path to a pem encoded CA bundle to validate the server certificate
* `Endpoint`: The address + port (without protocol) of the grpc server

TODO: The client can further be customized by providing [grpc.DialOption](https://pkg.go.dev/google.golang.org/grpc#DialOption) in the `grpc_client_options` value group.