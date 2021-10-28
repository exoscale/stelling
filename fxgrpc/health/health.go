package health

import (
	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

var Module = fx.Options(
	fx.Provide(health.NewServer),
	fx.Invoke(RegisterHealthService),
)

func RegisterHealthService(healthServer *health.Server, grpcServer *grpc.Server) {
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)
}
