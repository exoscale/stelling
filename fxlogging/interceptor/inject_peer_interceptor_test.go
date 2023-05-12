package interceptor

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/exoscale/stelling/fxgrpc/grpctest"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap/zaptest"
	pb "google.golang.org/grpc/examples/route_guide/routeguide"
	"google.golang.org/grpc/metadata"
)

type injectPeerRouteGuideServer struct {
	pb.UnimplementedRouteGuideServer
}

func newInjectPeerRouteGuideServer() pb.RouteGuideServer {
	return &injectPeerRouteGuideServer{}
}

func (s *injectPeerRouteGuideServer) GetFeature(ctx context.Context, req *pb.Point) (*pb.Feature, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, fmt.Errorf("No metadata in context")
	}
	ps := md.Get("peer.service")
	if len(ps) != 1 {
		return nil, fmt.Errorf("Incorrect number of peer.service sent: %d", len(ps))
	}
	if ps[0] == "" {
		return nil, fmt.Errorf("peer.service is empty")
	}
	return &pb.Feature{}, nil
}

func (s *injectPeerRouteGuideServer) RecordRoute(stream pb.RouteGuide_RecordRouteServer) error {
	ctx := stream.Context()
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return fmt.Errorf("No metadata in context")
	}
	ps := md.Get("peer.service")
	if len(ps) != 1 {
		return fmt.Errorf("Incorrect number of peer.service sent: %d", len(ps))
	}
	if ps[0] == "" {
		return fmt.Errorf("peer.service is empty")
	}
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return stream.SendAndClose(&pb.RouteSummary{})
}

func TestInjectPeerInterceptor(t *testing.T) {
	var client pb.RouteGuideClient

	logger := zaptest.NewLogger(t)

	app := fxtest.New(t, fx.Options(
		grpctest.Module,
		fx.Supply(logger),
		fx.Provide(
			newInjectPeerRouteGuideServer,
			pb.NewRouteGuideClient,
			fx.Annotate(
				NewInjectPeerUnaryClientInterceptor,
				fx.ResultTags(`group:"unary_client_interceptor"`),
			),
			fx.Annotate(
				NewInjectPeerStreamClientInterceptor,
				fx.ResultTags(`group:"stream_client_interceptor"`),
			),
		),
		fx.Invoke(
			pb.RegisterRouteGuideServer,
		),
		fx.Populate(&client),
	))

	defer app.RequireStart().RequireStop()

	t.Run("UnaryClientInterceptor should set the peer.service metadata", func(t *testing.T) {
		_, err := client.GetFeature(context.Background(), &pb.Point{})
		require.NoError(t, err, "Server did not find the peer.service metadata")
	})

	t.Run("UnaryStreamInterceptor should set the peer.service metadata", func(t *testing.T) {
		stream, err := client.RecordRoute(context.Background())
		require.NoError(t, err)

		require.NoError(t, stream.Send(&pb.Point{}))
		_, err = stream.CloseAndRecv()
		require.NoError(t, err, "Server did not find the peer.service metadata")
	})
}
