# Logging Module

This module provides configuration for [Ubers zap logger](https://pkg.go.dev/go.uber.org/zap).
Almost all other modules expect a `*zap.Logger` to be present, so this module is as 
close to mandatory as is possible.

All logging will be done on `stdout`. It is assumed that any further log management facilities are
provided by the underlying platform. There is no support for logging to a file, rotation or shipping
logs, etc.

All logs emitted by the fx system itself are also logged via the zap Logger. In development mode
all events are logged at Debug level, errors are logged at Error level.
In preproduction and production mode only Error events are logged.

## Components
The module lazily provides the following components:

* A `*zap.Logger`
* An adaptor which makes `fx` use the provided logger
* GrpcServerInterceptors that log all incoming requests
* GrpcClientInterceptors that log all requests made with the client
* GrpcServerInterceptors that embed a `*zap.Logger`, enriched with request metadata, in the context
* GrpcClientInterceptors that set `peer.service` metadata, which are logged by the server
* HttpMiddleware that logs all incoming requests on the server

## Options
* WithZapOption
  Allows passing in additional options that will be fed through to the `zap.Logger` constructor without
  modification.
* WithFxLoggerOption
  Allows you to further configure the logging of the fx system itself
* WithGrpcServerInterceptorOptions and WithGrpcClientInterceptorOptions
  Allows customization of the provided grpc interceptor loggers.
  See [interceptor.Option](https://pkg.go.dev/github.com/exoscale/stelling/fxlogging/interceptor#Option)
* WithHTTPInterceptorOptions
  Allows customization of the provided HTTP request logger.
  See [interceptor.HTTPOption](https://pkg.go.dev/github.com/exoscale/stelling/fxlogging/interceptor#HTTPOption)
* WithGrpcClientInterceptors
  By default the module supplies client logging interceptors. In certain high volume cases this is not
  desirable. With this option they can be removed from the system.


```go
fx.Supply(fx.Annotate(fxlogger.WithLogLevel(zapcore.InfoLevel), fx.ResultTags(`group:"fxlogger_opts"`)))
```

## Configuration file
At the moment the configuration for the logger only has a single option: `mode`:

* `development` (default): Uses zap's `Development` preset. Logs at `debug` level in a pretty printed format
* `production`: Uses zap's `Production` preset. Ensures timestamps are in UTC.
* `preproduction`: Same as `production`, but lowers level to `debug` and disables sampling.

All loggers print to stdout instead of stderr.

The settings behind each mode may be tuned further to suit the logging needs in each environment.
