package fxgrpc_test

import (
	"context"
	"fmt"

	sconfig "github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/fxgrpc"
	fxhttp "github.com/exoscale/stelling/fxhttp"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	pb "google.golang.org/grpc/examples/route_guide/routeguide"
	"google.golang.org/grpc/status"
)

type Config struct {
	fxhttp.Server
	fxgrpc.Client
}

type RouteGuideServer struct {
	pb.UnimplementedRouteGuideServer
}

func NewRouteGuideServer() pb.RouteGuideServer {
	return &RouteGuideServer{}
}

func Example() {
	conf := &Config{}
	args := []string{"grpc-test", "--server.address", "localhost:8080", "--client.endpoint", "localhost:8080", "--client.insecure-connection"}
	if err := sconfig.Load(conf, args); err != nil {
		panic(err)
	}
	opts := fx.Options(
		// Suppressing fx logs to ensure deterministic output
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		fxgrpc.NewServerModule(conf),
		fxgrpc.NewClientModule(conf),
		fx.Provide(
			// supplying a NopLogger to make output deterministic
			// In practise you'd use fxlogging.NewModule to get a zap logger
			// and replace the fxevent logger
			zap.NewNop,
			NewRouteGuideServer,
			pb.NewRouteGuideClient,
		),
		fx.Invoke(
			pb.RegisterRouteGuideServer,
			// We explicitly need to invoke this, because ordering matters
			fxgrpc.StartGrpcServer,
			run,
		),
	)
	if err := fx.ValidateApp(opts); err != nil {
		panic(err)
	}

	fx.New(opts).Run()

	// Output:
	// Endpoint returned status rpc error: code = Unimplemented desc = method GetFeature not implemented
}

func run(lc fx.Lifecycle, sd fx.Shutdowner, client pb.RouteGuideClient) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				defer sd.Shutdown() //nolint:errcheck
				_, err := client.GetFeature(
					context.Background(),
					&pb.Point{},
				)
				res, ok := status.FromError(err)
				if !ok {
					panic(fmt.Sprintln("could not extract grpc status code:", err))
				}
				fmt.Println("Endpoint returned status", res.String())
			}()
			return nil
		},
	})
}
