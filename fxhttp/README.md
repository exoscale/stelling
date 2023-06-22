# HTTP Module

This module provides [http server](https://pkg.go.dev/net/http) support.

> This module is still a work in progress. It's primary usage is to provide an HTTP server
  for use with other stelling modules. It provides no facilities to build a mux, nor any
  support for middleware. This will be added when we have daemons that have a need for these
  features and hopefully prevent us from building hard to use abstractions.

This package provides 2 modules:

* A regular module which provides a top level http server
* A "named" module, which provides a named submodule: this one is intended to be used in other stelling packages, such as metrics.
  It doesn't provide all the features of the main module. To create the main http server of your application you should use the regular module.

## Components 
The module lazily provides the following components:

* A `*http.Server`

The client `Invoke` `StartHttpServer` explicitly in his system.
This allows fine grained control over the start and stop order of components that do not share a dependency.

The module will also use `CertficateReloader` in case the configuration specifies TLS options.

## Configuration
The module provides the following configuration options:
* `Address`: The address + port on which the http server will bind
* `TLS`: A boolean indicating that the server must expose using TLS
* `CertFile`: Path to the pem encoded server TLS certificate
* `KeyFile`: Path to the pem encoded private key of the server TLS certificate
* `ClientCAFile`: Path to a pem encoded CA cert bundle used to validate clients. No client validation happens if unset.

