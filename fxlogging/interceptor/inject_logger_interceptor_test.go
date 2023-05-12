package interceptor

import (
	"context"
	"io"
	"testing"

	"github.com/exoscale/stelling/fxgrpc/grpctest"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"
	pb "google.golang.org/grpc/examples/route_guide/routeguide"
)

type injectLoggerRouteGuideServer struct {
	pb.UnimplementedRouteGuideServer
}

func newInjectLoggerRouteGuideServer() pb.RouteGuideServer {
	return &injectLoggerRouteGuideServer{}
}

func (s *injectLoggerRouteGuideServer) GetFeature(ctx context.Context, req *pb.Point) (*pb.Feature, error) {
	logger := LoggerFromContext(ctx)
	logger.Info("GetFeature")
	return &pb.Feature{}, nil
}

func (s *injectLoggerRouteGuideServer) ListFeatures(req *pb.Rectangle, stream pb.RouteGuide_ListFeaturesServer) error {
	logger := LoggerFromContext(stream.Context())
	logger.Info("ListFeatures")
	return stream.Send(&pb.Feature{})
}

func TestInjectLoggerInterceptor(t *testing.T) {
	var client pb.RouteGuideClient

	core, observer := observer.New(zapcore.DebugLevel)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.WrapCore(func(_ zapcore.Core) zapcore.Core { return core })))

	app := fxtest.New(t, fx.Options(
		grpctest.Module,
		fx.Supply(logger),
		fx.Provide(
			newInjectLoggerRouteGuideServer,
			pb.NewRouteGuideClient,
			fx.Annotate(
				NewInjectLoggerUnaryServerInterceptor,
				fx.ResultTags(`group:"unary_server_interceptor"`),
			),
			fx.Annotate(
				NewInjectLoggerStreamServerInterceptor,
				fx.ResultTags(`group:"stream_server_interceptor"`),
			),
		),
		fx.Invoke(
			pb.RegisterRouteGuideServer,
		),
		fx.Populate(&client),
	))
	defer app.RequireStart().RequireStop()

	t.Run("UnaryServerInterceptor should inject a configured logger in the context", func(t *testing.T) {
		_, err := client.GetFeature(context.Background(), &pb.Point{})
		require.NoError(t, err)

		logs := observer.TakeAll()
		require.Len(t, logs, 1)
		log := logs[0]
		require.Equal(t, "GetFeature", log.Message)
		require.NotEmpty(t, log.ContextMap()["otlp.trace_id"])
	})

	t.Run("StreamServerInterceptor should inject a configured logger in the context", func(t *testing.T) {
		stream, err := client.ListFeatures(context.Background(), &pb.Rectangle{})
		require.NoError(t, err)
		for {
			_, err := stream.Recv()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)

			logs := observer.TakeAll()
			require.Len(t, logs, 1)
			log := logs[0]
			require.Equal(t, "ListFeatures", log.Message)
			require.NotEmpty(t, log.ContextMap()["otlp.trace_id"])
		}
	})
}
