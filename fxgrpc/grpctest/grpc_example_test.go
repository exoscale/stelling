package grpctest_test

import (
	"context"
	"fmt"

	"github.com/exoscale/stelling/fxgrpc/grpctest"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	pb "google.golang.org/grpc/examples/route_guide/routeguide"
	"google.golang.org/grpc/status"
)

type RouteGuideServer struct {
	pb.UnimplementedRouteGuideServer
}

func NewRouteGuideServer() pb.RouteGuideServer {
	return &RouteGuideServer{}
}

func Example() {
	opts := fx.Options(
		// Suppressing fx logs to ensure deterministic output
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		grpctest.Module,
		fx.Provide(
			// supplying a NopLogger to make output deterministic
			zap.NewNop,
			NewRouteGuideServer,
			pb.NewRouteGuideClient,
		),
		fx.Invoke(
			pb.RegisterRouteGuideServer,
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
