package fxauthorizer_test

import (
	"context"
	"fmt"

	sconfig "github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/fxauthorizer"
	"github.com/exoscale/stelling/fxgrpc"
	"github.com/exoscale/stelling/fxgrpc/health"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	pb "google.golang.org/grpc/examples/route_guide/routeguide"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type Config struct {
	fxgrpc.Server
	fxgrpc.Client
	fxauthorizer.Authorizer
}

type RouteGuideServer struct {
	pb.UnimplementedRouteGuideServer
}

func NewRouteGuideServer() pb.RouteGuideServer {
	return &RouteGuideServer{}
}

func Example() {
	conf := &Config{}
	rule := "request.service == \"grpc.health.v1.Health\""
	args := []string{"authorizer-test", "--authorizer.rule", rule, "--server.address", "localhost:8080", "--client.endpoint", "localhost:8080", "--client.insecure-connection"}
	if err := sconfig.Load(conf, args); err != nil {
		panic(err)
	}
	opts := fx.Options(
		// Suppressing fx logs to ensure deterministic output
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		fxgrpc.NewServerModule(conf),
		fxgrpc.NewClientModule(conf),
		health.Module,
		fxauthorizer.NewModule(conf),
		fx.Provide(
			zap.NewNop,
			NewRouteGuideServer,
			pb.NewRouteGuideClient,
			healthpb.NewHealthClient,
		),
		fx.Invoke(
			pb.RegisterRouteGuideServer,
			fxgrpc.StartGrpcServer,
			run,
		),
	)
	if err := fx.ValidateApp(opts); err != nil {
		panic(err)
	}
	fx.New(opts).Run()

	// Output:
	// Endpoint returned status rpc error: code = PermissionDenied desc = authorization failed: policy denied
	// Healthcheck returned status:SERVING
}

func run(lc fx.Lifecycle, sd fx.Shutdowner, client pb.RouteGuideClient, healthClient healthpb.HealthClient) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				defer sd.Shutdown() //nolint:errcheck
				_, err := client.GetFeature(context.Background(), &pb.Point{})
				res, ok := status.FromError(err)
				if !ok {
					panic(fmt.Sprintln("could not extract grpc status code:", err))
				}
				fmt.Println("Endpoint returned status", res.String())

				resp, err := healthClient.Check(context.Background(), &healthpb.HealthCheckRequest{})
				if err != nil {
					panic(fmt.Sprintln("healthcheck request returned error:", err))
				}
				fmt.Println("Healthcheck returned", resp.String())
			}()
			return nil
		},
	})
}
