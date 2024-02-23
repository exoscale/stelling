# Zapgrpc

This package provides an adapter from the [GRPCLoggerV2 interface](https://pkg.go.dev/google.golang.org/grpc/grpclog#LoggerV2) to zap.

Zap already provides its [own adapter](https://pkg.go.dev/go.uber.org/zap/zapgrpc),
however this maps the GRPCLog Info level to the zap Info level.
When inspecting the actual logs, GRPC uses Info the way we use Debug.

Since the log level used by the upstream zap adapter is not configurable for the 
GRPCLogV2 interface, we had to fork and fix it.

This package has also removed support for the V1 logger and drops some tests around
`Fatal`, which leads to some LOC reduction.