# Metrics Module

This module provides [prometheus metrics](https://pkg.go.dev/github.com/prometheus/client_golang/prometheus) support.

This package provides 3 modules:

* A regular module that exposes a standard prometheus endpoint for use in long running daemons
* An OTLP module which uses the prometheus sdk to define metrics, but pushes them out over OTLP rather than the standard prometheus http endpoint
* A push module that pushes metrics into push gateway, for use with jobs

Because client code only interacts with the `*prometheus.Registry`, they can be swapped out transparently.

The following metric collectors are registered by default:
* [Go collector](https://pkg.go.dev/github.com/prometheus/client_golang@v1.14.0/prometheus/collectors#NewGoCollector) instrumenting the go runtime
* [Process collector](https://pkg.go.dev/github.com/prometheus/client_golang@v1.14.0/prometheus/collectors#NewProcessCollector) instrumenting the current process
* Version collector exposing the current git revision sha and timestamp using [go buildinfo](https://pkg.go.dev/runtime/debug#BuildInfo)

Additional custom metrics can of course be registered.

## Regular Module

### Components 
The module lazily provides the following components:

* A `*prometheus.Registry`
* GrpcServerInterceptors that count all incoming requests by method and status
* GrpcClientInterceptors that count all requests made with the client by method and status

It starts an additional webserver exposing the prometheus endpoint.

## OTLP Module

### Components 
The module lazily provides the following components:

* A `*prometheus.Registry`
* A `metric.MeterProvider` (allows you to define metrics with the otel sdk)
* GrpcServerInterceptors that count all incoming requests by method and status
* GrpcClientInterceptors that count all requests made with the client by method and status

It adds hooks to push the metrics when the system stops and at regular intervals during runtime.

### Configuration
The module provides the following configuration options:
* `GrpcClient`: A grpc client config, see the docs in the fxgrpc package for details
* `Histograms`: A bool which enables support for histograms in the grpc middleware (will most likely be removed)
* `ProcessName`: A string used as a prefix inside the process collector to prevent clashes
* `PushInterval`: The frequency at which metrics are pushed during runtime
* `Enabled`: Disables the pushing of metrics completely

## Push Module

### Components
The module lazily provides the following components:

* A `*prometheus.Registry`
* GrpcServerInterceptors that count all incoming requests by method and status
* GrpcClientInterceptors that count all requests made with the client by method and status

It adds hooks to push the metrics when the system stops and at regular intervals during runtime.

### Configuration
* `InsecureConnection`: Disables TLS when connecting to the endpoint
* `CertFile`: Path to a pem encoded client certificate
* `KeyFile`: Path to the pem encoded key of the client certificate
* `RootCAFile`: Path to a pem encoded bundle of CA certificates used to validate the server
* `Endpoint`: The http endpoint of the push gateway
* `Histograms`: A bool which enables support for histograms in the grpc middleware (will most likely be removed)
* `ProcessName`: A string used as a prefix inside the process collector to prevent clashes
* `JobName`: The name of the job in pushgateway
* `GroupingLabelKey`: The label on which pushgateway groups metrics
  Pushgateway keeps a copy of each metric for each value of GroupingLabelKey
* `GroupingLabelValue`: The value for this instance of the GroupingLabelKey
* `PushInterval`: The frequency at which metrics are pushed during runtime. 
  When `0` metrics are only pushed when the system stops
