# Grpc Reflection Module
This module installs the [grpc reflection service](https://github.com/grpc/grpc/blob/master/doc/server-reflection.md) on a grpc server instance.
It allows tools like [grpcurl](https://github.com/fullstorydev/grpcurl) to discover and interact with available services without needing access to the proto files.

It is sufficient to add this module to your system, next to the grpc server module to enable the functionality.

```go
app := fx.New(fx.Options(
    fxgrpc.NewServerModule(conf),
    reflection.Module,
    fx.Provide(NewMyServerImpl),
    fx.Invoke(
        pb.RegisterMyServer,
        fxgrpc.StartGrpcServer,
    ),
))
```