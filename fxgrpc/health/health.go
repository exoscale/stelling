// Package health provides client-side health check capabilities for grpc servers.
package health

import (
	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// Add a service that exposes the grpc server's health
var Module = fx.Module(
	"grpc-healthcheck",
	fx.Provide(health.NewServer),
	fx.Invoke(RegisterHealthService),
)

func RegisterHealthService(healthServer *health.Server, grpcServer *grpc.Server) {
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)
}
