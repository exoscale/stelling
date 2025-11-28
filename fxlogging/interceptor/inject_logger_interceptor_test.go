package interceptor

import (
	"context"
	"io"
	"regexp"
	"slices"
	"testing"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"github.com/exoscale/stelling/fxgrpc"
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
	var defaultGRPCFields = []string{
		"otlp.trace_id",
		"rpc.system",
		"service.name",
		"rpc.method",
		"rpc.service",
	}

	core, observer := observer.New(zapcore.DebugLevel)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.WrapCore(func(_ zapcore.Core) zapcore.Core { return core })))

	app := fxtest.New(t, fx.Options(
		grpctest.Module,
		fx.Supply(logger),
		fx.Provide(
			newInjectLoggerRouteGuideServer,
			pb.NewRouteGuideClient,
			fx.Annotate(
				func(logger *zap.Logger) *fxgrpc.UnaryServerInterceptor {
					return &fxgrpc.UnaryServerInterceptor{Weight: 42, Interceptor: NewInjectLoggerUnaryServerInterceptor(logger)}
				},
				fx.ResultTags(`group:"unary_server_interceptor"`),
			),
			fx.Annotate(
				func(logger *zap.Logger) *fxgrpc.StreamServerInterceptor {
					return &fxgrpc.StreamServerInterceptor{Weight: 42, Interceptor: NewInjectLoggerStreamServerInterceptor(logger)}
				},
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
		require.NotEmpty(t, log.ContextMap()["rpc.method"])
		require.NotEmpty(t, log.ContextMap()["rpc.service"])
		require.NotEmpty(t, log.ContextMap()["rpc.system"])
		require.NotEmpty(t, log.ContextMap()["service.name"])
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
		}
		logs := observer.TakeAll()
		require.Len(t, logs, 1)
		log := logs[0]
		require.Equal(t, "ListFeatures", log.Message)
		require.NotEmpty(t, log.ContextMap()["otlp.trace_id"])
		require.NotEmpty(t, log.ContextMap()["rpc.method"])
		require.NotEmpty(t, log.ContextMap()["rpc.service"])
		require.NotEmpty(t, log.ContextMap()["rpc.system"])
		require.NotEmpty(t, log.ContextMap()["service.name"])
	})

	t.Run("UnaryServerInterceptor should set only default gRPC fields", func(t *testing.T) {
		_, err := client.GetFeature(context.Background(), &pb.Point{})
		require.NoError(t, err)
		_, err = client.GetFeature(context.Background(), &pb.Point{})
		require.NoError(t, err)

		logs := observer.TakeAll()
		require.Len(t, logs, 2)
		log := logs[1]
		require.Equal(t, "GetFeature", log.Message)
		defaultFields := []zapcore.Field{}
		for _, field := range log.Context {
			if slices.Contains(defaultGRPCFields, field.Key) {
				defaultFields = append(defaultFields, field)
			}
		}
		require.Len(t, defaultFields, 5)
	})

	t.Run("StreamServerInterceptor should set only default gRPC fields", func(t *testing.T) {
		stream, err := client.ListFeatures(context.Background(), &pb.Rectangle{})
		require.NoError(t, err)
		for {
			_, err := stream.Recv()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
		}
		stream2, err := client.ListFeatures(context.Background(), &pb.Rectangle{})
		require.NoError(t, err)
		for {
			_, err := stream2.Recv()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
		}
		logs := observer.TakeAll()
		require.Len(t, logs, 2)
		log := logs[1]
		require.Equal(t, "ListFeatures", log.Message)
		defaultFields := []zapcore.Field{}
		for _, field := range log.Context {
			if slices.Contains(defaultGRPCFields, field.Key) {
				defaultFields = append(defaultFields, field)
			}
		}
		require.Len(t, defaultFields, 5)
	})
}

func TestInjectLoggerInterceptor_WithMetadataFields(t *testing.T) {
	var client pb.RouteGuideClient

	core, observer := observer.New(zapcore.DebugLevel)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.WrapCore(func(_ zapcore.Core) zapcore.Core { return core })))

	rid := uuid.New().String()

	app := fxtest.New(t, fx.Options(
		grpctest.Module,
		fx.Supply(logger),
		fx.Provide(
			newInjectLoggerRouteGuideServer,
			pb.NewRouteGuideClient,
			fx.Annotate(
				func(logger *zap.Logger) *fxgrpc.UnaryServerInterceptor {
					return &fxgrpc.UnaryServerInterceptor{Weight: 42, Interceptor: NewInjectLoggerUnaryServerInterceptor(logger, WithMetadataFields(map[string]string{"x-request-id": "request_id"}))}
				},
				fx.ResultTags(`group:"unary_server_interceptor"`),
			),
			fx.Annotate(
				func(logger *zap.Logger) *fxgrpc.StreamServerInterceptor {
					return &fxgrpc.StreamServerInterceptor{Weight: 42, Interceptor: NewInjectLoggerStreamServerInterceptor(logger, WithMetadataFields(map[string]string{"x-request-id": "request_id"}))}
				},
				fx.ResultTags(`group:"stream_server_interceptor"`),
			),
		),
		fx.Invoke(
			pb.RegisterRouteGuideServer,
		),
		fx.Populate(&client),
	))
	defer app.RequireStart().RequireStop()

	t.Run("Unary should extract request-id from metadata", func(t *testing.T) {
		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-request-id", rid))
		_, err := client.GetFeature(ctx, &pb.Point{})
		require.NoError(t, err)

		logs := observer.TakeAll()
		require.Len(t, logs, 1)
		log := logs[0]
		require.Equal(t, "GetFeature", log.Message)
		require.Equal(t, rid, log.ContextMap()["request_id"])
	})

	t.Run("Stream should extract request-id from metadata", func(t *testing.T) {
		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-request-id", rid))
		stream, err := client.ListFeatures(ctx, &pb.Rectangle{})
		require.NoError(t, err)
		for {
			_, err := stream.Recv()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
		}
		logs := observer.TakeAll()
		require.Len(t, logs, 1)
		log := logs[0]
		require.Equal(t, "ListFeatures", log.Message)
		require.Equal(t, rid, log.ContextMap()["request_id"])
	})
}

func TestInjectLoggerInterceptor_WithExtraFieldsFunc(t *testing.T) {
	var client pb.RouteGuideClient

	extraValue := "extra_value"

	extraFields := func(logger *zap.Logger, info *otelgrpc.InterceptorInfo, payload any) *zap.Logger {
		var server string
		switch info.Type {
		case otelgrpc.StreamServer:
			server = "stream"
		case otelgrpc.UnaryServer:
			server = "unary"
		}

		return logger.With(
			zap.String("extra_field", extraValue),
			zap.Any("payload", payload.(proto.Message)),
			zap.String("server_type", server),
		)
	}

	core, observer := observer.New(zapcore.DebugLevel)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.WrapCore(func(_ zapcore.Core) zapcore.Core { return core })))

	app := fxtest.New(t, fx.Options(
		grpctest.Module,
		fx.Supply(logger),
		fx.Provide(
			newInjectLoggerRouteGuideServer,
			pb.NewRouteGuideClient,
			fx.Annotate(
				func(logger *zap.Logger) *fxgrpc.UnaryServerInterceptor {
					return &fxgrpc.UnaryServerInterceptor{Weight: 42, Interceptor: NewInjectLoggerUnaryServerInterceptor(logger, WithExtraFieldsFunc(extraFields))}
				},
				fx.ResultTags(`group:"unary_server_interceptor"`),
			),
			fx.Annotate(
				func(logger *zap.Logger) *fxgrpc.StreamServerInterceptor {
					return &fxgrpc.StreamServerInterceptor{Weight: 42, Interceptor: NewInjectLoggerStreamServerInterceptor(logger, WithExtraFieldsFunc(extraFields))}
				},
				fx.ResultTags(`group:"stream_server_interceptor"`),
			),
		),
		fx.Invoke(
			pb.RegisterRouteGuideServer,
		),
		fx.Populate(&client),
	))
	defer app.RequireStart().RequireStop()

	t.Run("Unary should extract all extra fields", func(t *testing.T) {
		_, err := client.GetFeature(context.Background(), &pb.Point{Latitude: 1, Longitude: 2})
		require.NoError(t, err)

		logs := observer.TakeAll()
		require.Len(t, logs, 1)
		log := logs[0]
		require.Equal(t, "GetFeature", log.Message)
		require.Regexp(t, regexp.MustCompile(`latitude:1[ ]*longitude:2`), log.ContextMap()["payload"])
		require.Equal(t, extraValue, log.ContextMap()["extra_field"])
		require.Equal(t, "unary", log.ContextMap()["server_type"])
	})
}
