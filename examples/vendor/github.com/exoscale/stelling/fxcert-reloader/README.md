# CertReloader Module

This module provides facilities for automatically reloading TLS certificates.

It will generally not be used directly, but through other stelling modules such as the grpc server and clients.


## Components 
Due to the simple nature of this package, there is no module constructor.
There is a `ProvideCertReloader` function which will provision a `CertReloader` and register lifecycle hooks.


## Configuration
The module provides the following configuration options:
* `CertFile`: Path to the pem encoded server TLS certificate
* `KeyFile`: Path to the pem encoded private key of the server TLS certificate
* `ReloadInterval`: The minimum time between 2 certificate reloads

