# Pprof Module

This module provides turnkey [pprof](https://pkg.go.dev/runtime/pprof) support.

## Components
This module does not provide any components to the system.
It will instrument the runtime, either via an http server or by enabling
profiling for the entire runtime of the process.

## Configuration
The module provides 2 options:
* `Enabled`: When `true`, a webserver will spawn that exposes the [pprof http endpoints](https://pkg.go.dev/net/http/pprof)
  By default it will bind to `localhost:9092`, but the parameters can be overwritten by the embedded http module config
* `GenerateFiles`: When this is set to a directory, it will profile the entire runtime of the process.
  The profile information will be saved in the directory as `pprof.cpu` and `pprof.mem`.
  Disables the http server, even if `Enabled` is set to true.

When neither option are set, the constructor will add no functions to the system.