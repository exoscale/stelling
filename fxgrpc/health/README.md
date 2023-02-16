# Grpc Health Module
This module installs the default implementation of the [grpc health service](https://github.com/grpc/grpc/blob/master/doc/health-checking.md) on a grpc server instance.
It will return status `SERVING` for any incoming requests and performs no additional checks.

Systems that want to provide more detailed health checks will have to provide their own implementation of the health check service.
It can be registered on the grpc server like any other service.

It is sufficient to add this module to your system, next to the grpc server module to enable the functionality.

```go
app := fx.New(fx.Options(
    fxgrpc.NewServerModule(conf),
    health.Module,
    fx.Provide(NewMyServerImpl),
    fx.Invoke(
        pb.RegisterMyServer,
        fxgrpc.StartGrpcServer,
    ),
))
```