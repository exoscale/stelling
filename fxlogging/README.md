# Logging Module

This module provides configuration for [Ubers zap logger](https://pkg.go.dev/go.uber.org/zap).
Almost all other modules expect a `*zap.Logger` to be present, so this module is as 
close to mandatory as is possible.

## Components
The module lazily provides the following components:

* A `*zap.Logger`
* An adaptor which makes fx use the provided logger
* GrpcServerInterceptors that log all incoming requests
* GrpcClientInterceptors that log all requests made with the client

In case special configuration of the zap Logger is needed, that is not supported by the exposed
`LoggingConfig`, a [value group](https://uber-go.github.io/fx/value-groups/) of `zap.Option` with name
`zap_opts` can be inserted into the system: these will be fed through to the `zap.Logger` constructor
without modification. The included example test provides a working example of this.

Similarly the grpc server and client interceptors can be customized by supplying a value group of
[grpc_zap.Option](https://pkg.go.dev/github.com/grpc-ecosystem/go-grpc-middleware/logging/zap#Option)
with the name `grpc_zap_server_options` and `grpc_zap_client_options` respectively.
This allows customization of the grpc code to log level mapping and passing in a custom decider for when
requests should be logged.

## Configuration file
At the moment the configuration for the logger only has a single option: `mode`:

* `development` (default): Uses zap's `Development` preset. Logs at `debug` level in a pretty printed format
* `production`: Uses zap's `Production` preset. Ensures timestamps are in UTC.
* `preproduction`: Same as `production`, but lowers level to `debug` and disables sampling.

All loggers print to stdout instead of stderr.

The settings behind each mode may be tuned further to suit the logging needs in each environment.