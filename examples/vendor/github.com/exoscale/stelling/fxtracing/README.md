# Tracing Module

This module provides [opentelemetry tracing](https://pkg.go.dev/go.opentelemetry.io/otel/trace) support.

## Components
The module lazily provides the following components:

* A `trace.TracerProvider`
* GrpcServerInterceptors that traces all incoming requests
* GrpcClientInterceptors that traces all requests made with the client

At the moment we do not expose any advanced otelgrpc, or samplerprovider options.

## Configuration
The module provides the following configuration options:
* `Enabled`: Turns tracing support on or off. When turned off, a `NopTracerProvider` is inserted into the system.
* `InsecureConnection`: Disables TLS when connecting to the tracing endpoint
* `CertFile`: Path to a pem encoded client TLS certificate
* `KeyFile`: Path to the pem encoded private key of the client TLS certificate
* `RootCAFile`: Path to a pem encoded CA bundle to validate the server certificate
* `Endpoint`: The address + port (without protocol) of the grpc service where spans should be delivered
  When it is `""` and `InsecureConnection` is set, spans will be printed to `stdout`