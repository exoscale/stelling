package grpctest_test

import (
	"context"
	"fmt"
	"io"

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

func (s *RouteGuideServer) RouteChat(stream pb.RouteGuide_RouteChatServer) error {
	for {
		note, err := stream.Recv()
		switch {
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		}

		response := &pb.RouteNote{
			Message:  "Echo: " + note.Message,
			Location: note.Location,
		}

		if err := stream.Send(response); err != nil {
			return err
		}
	}
}

// Example tests an unimplemented method rpc error
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

// Example_rpcWithNilPoint verifies input validation from client to server and
// ensures the server handles nil requests safely
func Example_rpcWithNilPoint() {
	opts := fx.Options(
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		grpctest.Module,
		fx.Provide(
			zap.NewNop,
			NewRouteGuideServer,
			pb.NewRouteGuideClient,
		),
		fx.Invoke(
			pb.RegisterRouteGuideServer,
			func(lc fx.Lifecycle, sd fx.Shutdowner, client pb.RouteGuideClient) {
				lc.Append(fx.Hook{
					OnStart: func(context.Context) error {
						go func() {
							defer sd.Shutdown()
							_, err := client.GetFeature(context.Background(), nil)
							if err != nil {
								fmt.Println("GetFeature with nil input:", err != nil)
							}
						}()
						return nil
					},
				})
			},
		),
	)
	if err := fx.ValidateApp(opts); err != nil {
		panic(err)
	}
	fx.New(opts).Run()

	// Output:
	// GetFeature with nil input: true
}

// Example_rpcRouteChat showcase the usage of the RouteChat gRPC method.
// It sends multiple RouteNote messages to the server, then receives the server's
// responses and prints the echoed messages back to the client.
// This showcases a bidirectional streaming RPC, where the client sends and receives messages
// using a gRPC stream.
func Example_rpcRouteChat() {
	opts := fx.Options(
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		grpctest.Module,
		fx.Provide(
			zap.NewNop,
			NewRouteGuideServer,
			pb.NewRouteGuideClient,
		),
		fx.Invoke(
			pb.RegisterRouteGuideServer,
			func(lc fx.Lifecycle, sd fx.Shutdowner, client pb.RouteGuideClient) {
				lc.Append(fx.Hook{
					OnStart: func(ctx context.Context) error {
						go func() {
							defer sd.Shutdown()

							stream, err := client.RouteChat(context.Background())
							if err != nil {
								fmt.Println("failed to open RouteChat:", err)
								return
							}

							notes := []*pb.RouteNote{
								{Message: "exo1", Location: &pb.Point{Latitude: 48856400, Longitude: -1224192}},
								{Message: "exo2", Location: &pb.Point{Latitude: -48856400, Longitude: 1224192}},
							}

							for _, note := range notes {
								if err := stream.Send(note); err != nil {
									fmt.Println("send error:", err)
									return
								}
								resp, err := stream.Recv()
								if err != nil {
									fmt.Println("recv error:", err)
									return
								}
								fmt.Println("Received:", resp.Message)
							}

							_ = stream.CloseSend()
						}()
						return nil
					},
				})
			},
		),
	)

	if err := fx.ValidateApp(opts); err != nil {
		panic(err)
	}
	fx.New(opts).Run()

	// Output:
	// Received: Echo: exo1
	// Received: Echo: exo2
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
