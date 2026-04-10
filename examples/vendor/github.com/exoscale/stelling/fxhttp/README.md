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

You must supply a single `http.Handler` in the system, which will be set as the servers handler.
This way, the module does not enforce any opinions on how the server mux is constructed.

The client `Invoke`s `StartHttpServer` explicitly in his system.
This allows fine grained control over the start and stop order of components that do not share a dependency.

The module will also use `CertficateReloader` in case the configuration specifies TLS options.

## Options
* WithRPCMethodMapper
  Passes in a function which returns the name of the rpc method a particular request is calling.
  This information will be added as metadata to the observability
* WithServerModuleName
  This option allows creating additional "named" http servers. It is intended to be used by other stelling
  modules, such as the metrics. Named servers do not provide all the features of the main server: for example
  no middleware will be installed.j

## Configuration
The module provides the following configuration options:
* `Address`: The address + port on which the http server will bind
* `TLS`: A boolean indicating that the server must expose using TLS
* `CertFile`: Path to the pem encoded server TLS certificate
* `KeyFile`: Path to the pem encoded private key of the server TLS certificate
* `ClientCAFile`: Path to a pem encoded CA cert bundle used to validate clients. No client validation happens if unset.

