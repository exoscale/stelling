# Grpctest Module

The grpctest module can replace the fxgrpc Server and Client modules in a testing context.
Rather than relying on a network connection, the `*grpc.Server` and `grpc.ClientConnInterface` provided by this module are connected via an in memory buffer.

In addition to lower resource usage, it also increases the robustness of the tests because it has less requirements on the host which is running your tests: eg no port needs to be allocated.

If any other components in your test system supply middleware, they will be installed on the provided server and client.

You can compare the provided example test with the example test in the fxgrpc package.

## Components
The module lazily provides the following components:

* A `*grpc.Server`
* A `grpc.ClientConnInterface`

Both components are connected through a [bufcon](https://pkg.go.dev/google.golang.org/grpc/test/bufconn)