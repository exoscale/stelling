# Logging Interceptors

This package contains a number of logging related gRPC interceptors.

## Logging Interceptor
There are a number of alternatives out there.
We chose to write a custom one for the following reasons:

* All fields should adhere to the otel semantic convention.
  If a piece of data is present in both the logs and the trace, they should use the same name.
* Optionally log the rpc request in the same log line.
  This also means it does not support logging all messages on a stream.
  You will have to use another interceptor for that.
* Easily configure if a request should be logged or not.
* Correctly handle client streams

## Inject Logger Interceptor
This (server) interceptor injects a logger on the context, which is configured with request
specific parameters (current the `trace-id`).
The handler code can then extract the logger from the context.

## Inject Peer Interceptor
This (client) interceptor sets a `peer.service` metadata parameter. The value of this
is set to the opentelemetry `service.name` of the application which makes the call.
The logging interceptor is configured to add this to the request log.