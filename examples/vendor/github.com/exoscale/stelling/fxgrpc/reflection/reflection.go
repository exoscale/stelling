// Package reflection provides reflection capabilities for grpc servers.
package reflection

import (
	"go.uber.org/fx"
	"google.golang.org/grpc/reflection"
)

// Add a service that exposes the grpc server proto definition
var Module = fx.Module(
	"grpc-reflection",
	fx.Invoke(Register),
)

func Register(s reflection.GRPCServer) {
	reflection.Register(s)
}
