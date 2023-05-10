package interceptor

import (
	"context"
	"strings"
	"testing"

	"github.com/exoscale/stelling/fxgrpc/grpctest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc/codes"
	pb "google.golang.org/grpc/examples/route_guide/routeguide"
	"google.golang.org/grpc/status"
)

func newUnimplementedRouteGuideServer() pb.RouteGuideServer {
	return &pb.UnimplementedRouteGuideServer{}
}

func withTestSystem(t *testing.T, cb func(client pb.RouteGuideClient, logs *observer.ObservedLogs), opts ...fx.Option) {
	t.Helper()
	var client pb.RouteGuideClient

	core, logs := observer.New(zapcore.DebugLevel)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.WrapCore(func(_ zapcore.Core) zapcore.Core { return core })))

	app := fxtest.New(t, fx.Options(
		grpctest.Module,
		fx.Supply(logger),
		fx.Provide(
			newUnimplementedRouteGuideServer,
			pb.NewRouteGuideClient,
		),
		fx.Invoke(pb.RegisterRouteGuideServer),
		fx.Populate(&client),
		fx.Options(opts...),
	))
	defer app.RequireStart().RequireStop()

	cb(client, logs)
}

func TestLoggingServerInterceptor(t *testing.T) {
	t.Run("Should log unary requests on the server", func(t *testing.T) {
		run := func(client pb.RouteGuideClient, logs *observer.ObservedLogs) {
			_, err := client.GetFeature(context.Background(), &pb.Point{})
			require.Error(t, err)
			require.Equal(t, codes.Unimplemented, status.Code(err))
			require.Equal(t, 1, logs.Len())
			log := logs.AllUntimed()[0]

			require.Equal(t, zapcore.ErrorLevel, log.Level)
			require.Equal(t, "finished call", log.Message)
			// Since we know the logging logic is shared by all interceptors
			// We'll do this detailed check only once
			fields := log.ContextMap()
			require.Contains(t, fields, "rpc.system")
			require.Equal(t, "grpc", fields["rpc.system"])
			require.Contains(t, fields, "service.name")
			require.Equal(t, "unknown_service", fields["service.name"])
			require.Contains(t, fields, "rpc.method")
			require.Equal(t, "GetFeature", fields["rpc.method"])
			require.Contains(t, fields, "rpc.service")
			require.Equal(t, "routeguide.RouteGuide", fields["rpc.service"])
			require.Contains(t, fields, "rpc.request.start_time")
			require.Contains(t, fields, "rpc.grpc.status_code")
			require.Equal(t, codes.Unimplemented.String(), fields["rpc.grpc.status_code"])
			require.Contains(t, fields, "rpc.request.duration")
			require.Contains(t, fields, "sock.net.peer.address")
			require.Equal(t, "bufconn", fields["sock.net.peer.address"])
			require.Contains(t, fields, "otlp.trace_id")
			require.True(t, strings.HasPrefix(fields["otlp.trace_id"].(string), "local-"))
		}
		extraOpts := fx.Provide(
			fx.Annotate(
				NewLoggingUnaryServerInterceptor,
				fx.ResultTags(`group:"unary_server_interceptor"`),
			),
		)
		withTestSystem(t, run, extraOpts)
	})

	t.Run("Should log stream requests on the server", func(t *testing.T) {
		run := func(client pb.RouteGuideClient, logs *observer.ObservedLogs) {
			stream, err := client.ListFeatures(context.Background(), &pb.Rectangle{})
			require.NoError(t, err)
			_, err = stream.Recv()
			require.Error(t, err)
			require.Equal(t, codes.Unimplemented, status.Code(err))

			require.Equal(t, 1, logs.Len())
			log := logs.AllUntimed()[0]
			require.Equal(t, zapcore.ErrorLevel, log.Level)
			require.Equal(t, "finished call", log.Message)
		}
		extraOpts := fx.Provide(
			fx.Annotate(
				NewLoggingStreamServerInterceptor,
				fx.ResultTags(`group:"stream_server_interceptor"`),
			),
		)
		withTestSystem(t, run, extraOpts)
	})

	t.Run("Should log unary requests on the client", func(t *testing.T) {
		run := func(client pb.RouteGuideClient, logs *observer.ObservedLogs) {
			_, err := client.GetFeature(context.Background(), &pb.Point{})
			require.Error(t, err)
			require.Equal(t, codes.Unimplemented, status.Code(err))

			require.Equal(t, 1, logs.Len())
			log := logs.AllUntimed()[0]
			require.Equal(t, zapcore.ErrorLevel, log.Level)
			require.Equal(t, "finished call", log.Message)
		}
		extraOpts := fx.Provide(
			fx.Annotate(
				NewLoggingUnaryClientInterceptor,
				fx.ResultTags(`group:"unary_client_interceptor"`),
			),
		)
		withTestSystem(t, run, extraOpts)
	})

	t.Run("Should log stream requests on the client", func(t *testing.T) {
		run := func(client pb.RouteGuideClient, logs *observer.ObservedLogs) {
			stream, err := client.RecordRoute(context.Background())
			require.NoError(t, err)

			require.NoError(t, stream.Send(&pb.Point{}))
			_, err = stream.CloseAndRecv()
			require.Error(t, err)
			require.Equal(t, codes.Unimplemented, status.Code(err))

			require.Equal(t, 1, logs.Len())
			log := logs.AllUntimed()[0]
			require.Equal(t, zapcore.ErrorLevel, log.Level)
			require.Equal(t, "finished call", log.Message)
		}
		extraOpts := fx.Provide(
			fx.Annotate(
				NewLoggingStreamClientInterceptor,
				fx.ResultTags(`group:"stream_client_interceptor"`),
			),
		)
		withTestSystem(t, run, extraOpts)
	})

	t.Run("Should use custom codeToLevel function", func(t *testing.T) {
		run := func(client pb.RouteGuideClient, logs *observer.ObservedLogs) {
			_, err := client.GetFeature(context.Background(), &pb.Point{})
			require.Error(t, err)
			require.Equal(t, codes.Unimplemented, status.Code(err))

			require.Equal(t, 1, logs.Len())
			log := logs.AllUntimed()[0]
			require.Equal(t, zapcore.WarnLevel, log.Level)
			require.Equal(t, "finished call", log.Message)
		}
		extraOpts := fx.Provide(
			func() []Option {
				codeToLevel := func(code codes.Code) zapcore.Level {
					return zapcore.WarnLevel
				}
				return []Option{WithLevelFunc(codeToLevel)}
			},
			fx.Annotate(
				NewLoggingUnaryServerInterceptor,
				fx.ResultTags(`group:"unary_server_interceptor"`),
			),
		)
		withTestSystem(t, run, extraOpts)
	})

	t.Run("Should use custom logFilter", func(t *testing.T) {
		run := func(client pb.RouteGuideClient, logs *observer.ObservedLogs) {
			_, err := client.GetFeature(context.Background(), &pb.Point{})
			require.Error(t, err)
			require.Equal(t, codes.Unimplemented, status.Code(err))

			require.Zero(t, logs.Len())
		}
		extraOpts := fx.Provide(
			func() []Option {
				logFilter := func(_ *otelgrpc.InterceptorInfo) bool {
					return false
				}
				return []Option{WithLogFilter(logFilter)}
			},
			fx.Annotate(
				NewLoggingUnaryServerInterceptor,
				fx.ResultTags(`group:"unary_server_interceptor"`),
			),
		)
		withTestSystem(t, run, extraOpts)
	})

	t.Run("Should use custom payloadFilter", func(t *testing.T) {
		run := func(client pb.RouteGuideClient, logs *observer.ObservedLogs) {
			_, err := client.GetFeature(context.Background(), &pb.Point{})
			require.Error(t, err)
			require.Equal(t, codes.Unimplemented, status.Code(err))

			require.Equal(t, 1, logs.Len())
			log := logs.AllUntimed()[0]
			require.Equal(t, zapcore.ErrorLevel, log.Level)
			require.Equal(t, "finished call", log.Message)
			require.Contains(t, log.ContextMap(), "rpc.request.content")
		}
		extraOpts := fx.Provide(
			func() []Option {
				payloadFilter := func(_ *otelgrpc.InterceptorInfo) bool {
					return true
				}
				return []Option{WithPayloadFilter(payloadFilter)}
			},
			fx.Annotate(
				NewLoggingUnaryServerInterceptor,
				fx.ResultTags(`group:"unary_server_interceptor"`),
			),
		)
		withTestSystem(t, run, extraOpts)
	})
}
