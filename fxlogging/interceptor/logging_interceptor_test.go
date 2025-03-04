package interceptor

import (
	"context"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/exoscale/stelling/fxgrpc"
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

type loggingRouteGuideServer struct {
	pb.UnimplementedRouteGuideServer
}

func newLoggingRouteGuideServer() pb.RouteGuideServer {
	return &loggingRouteGuideServer{}
}

func (s *loggingRouteGuideServer) ListFeatures(req *pb.Rectangle, stream pb.RouteGuide_ListFeaturesServer) error {
	return stream.Send(&pb.Feature{})
}

func (s *loggingRouteGuideServer) RecordRoute(stream pb.RouteGuide_RecordRouteServer) error {
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

func withTestSystem(t *testing.T, cb func(client pb.RouteGuideClient, logs *observer.ObservedLogs), opts ...fx.Option) {
	t.Helper()
	var client pb.RouteGuideClient

	core, logs := observer.New(zapcore.DebugLevel)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.WrapCore(func(_ zapcore.Core) zapcore.Core { return core })))

	app := fxtest.New(t, fx.Options(
		grpctest.Module,
		fx.Supply(logger),
		fx.Provide(
			newLoggingRouteGuideServer,
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
			// The exact value relies on autodetection and isn't really fixed depending on how the ci is run
			//require.Equal(t, "interceptor.test", fields["service.name"])
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
				func(logger *zap.Logger, opts ...Option) *fxgrpc.UnaryServerInterceptor {
					return &fxgrpc.UnaryServerInterceptor{Weight: 42, Interceptor: NewLoggingUnaryServerInterceptor(logger, opts...)}
				},
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
			require.NoError(t, err)
			_, err = stream.Recv()
			require.Equal(t, io.EOF, err)

			require.Equal(t, 1, logs.Len())
			log := logs.AllUntimed()[0]
			require.Equal(t, zapcore.InfoLevel, log.Level)
			require.Equal(t, "finished call", log.Message)
		}
		extraOpts := fx.Provide(
			fx.Annotate(
				func(logger *zap.Logger, opts ...Option) *fxgrpc.StreamServerInterceptor {
					return &fxgrpc.StreamServerInterceptor{Weight: 42, Interceptor: NewLoggingStreamServerInterceptor(logger, opts...)}
				},
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
				func(logger *zap.Logger, opts ...Option) *fxgrpc.UnaryClientInterceptor {
					return &fxgrpc.UnaryClientInterceptor{Weight: 42, Interceptor: NewLoggingUnaryClientInterceptor(logger, opts...)}
				},
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
			require.NoError(t, err)

			require.Equal(t, 1, logs.Len())
			log := logs.AllUntimed()[0]
			require.Equal(t, zapcore.DebugLevel, log.Level)
			require.Equal(t, "finished call", log.Message)
		}
		extraOpts := fx.Provide(
			fx.Annotate(
				func(logger *zap.Logger, opts ...Option) *fxgrpc.StreamClientInterceptor {
					return &fxgrpc.StreamClientInterceptor{Weight: 42, Interceptor: NewLoggingStreamClientInterceptor(logger, opts...)}
				},
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
				codeToLevel := func(_ *otelgrpc.InterceptorInfo, code codes.Code) zapcore.Level {
					return zapcore.WarnLevel
				}
				return []Option{WithLevelFunc(codeToLevel)}
			},
			fx.Annotate(
				func(logger *zap.Logger, opts ...Option) *fxgrpc.UnaryServerInterceptor {
					return &fxgrpc.UnaryServerInterceptor{Weight: 42, Interceptor: NewLoggingUnaryServerInterceptor(logger, opts...)}
				},
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
				func(logger *zap.Logger, opts ...Option) *fxgrpc.UnaryServerInterceptor {
					return &fxgrpc.UnaryServerInterceptor{Weight: 42, Interceptor: NewLoggingUnaryServerInterceptor(logger, opts...)}
				},
				fx.ResultTags(`group:"unary_server_interceptor"`),
			),
		)
		withTestSystem(t, run, extraOpts)
	})

	t.Run("Should use custom payloadFilter", func(t *testing.T) {
		run := func(client pb.RouteGuideClient, logs *observer.ObservedLogs) {
			_, err := client.GetFeature(context.Background(), &pb.Point{
				Latitude:  12345,
				Longitude: 12345,
			})
			require.Error(t, err)
			require.Equal(t, codes.Unimplemented, status.Code(err))

			require.Equal(t, 1, logs.Len())
			log := logs.AllUntimed()[0]
			require.Equal(t, zapcore.ErrorLevel, log.Level)
			require.Equal(t, "finished call", log.Message)
			require.Contains(t, log.ContextMap(), "rpc.request.content")
			// Using a regex because zap.Any will use the String() implementation of the proto field, which is not stable
			require.Regexp(t, regexp.MustCompile(`latitude:12345[ ]*longitude:12345`), log.ContextMap()["rpc.request.content"])
		}
		extraOpts := fx.Provide(
			func() []Option {
				payloadFilter := func(_ *otelgrpc.InterceptorInfo) bool {
					return true
				}
				return []Option{WithPayloadFilter(payloadFilter)}
			},
			fx.Annotate(
				func(logger *zap.Logger, opts ...Option) *fxgrpc.UnaryServerInterceptor {
					return &fxgrpc.UnaryServerInterceptor{Weight: 42, Interceptor: NewLoggingUnaryServerInterceptor(logger, opts...)}
				},
				fx.ResultTags(`group:"unary_server_interceptor"`),
			),
		)
		withTestSystem(t, run, extraOpts)
	})

	t.Run("Should log correct payload with serverside stream", func(t *testing.T) {
		run := func(client pb.RouteGuideClient, logs *observer.ObservedLogs) {
			stream, err := client.ListFeatures(context.Background(), &pb.Rectangle{
				Lo: &pb.Point{Latitude: -1, Longitude: -1},
				Hi: &pb.Point{Latitude: 1, Longitude: 1},
			})
			require.NoError(t, err)
			_, err = stream.Recv()
			require.NoError(t, err)
			_, err = stream.Recv()
			require.Error(t, err)

			require.Equal(t, 1, logs.Len())
			log := logs.AllUntimed()[0]
			require.Equal(t, zapcore.InfoLevel, log.Level)
			require.Equal(t, "finished call", log.Message)
			require.Contains(t, log.ContextMap(), "rpc.request.content")
			// Using a regex because zap.Any will use the String() implementation of the proto field, which is not stable
			require.Regexp(t, regexp.MustCompile(`lo:{latitude:-1[ ]*longitude:-1}[ ]*hi:{latitude:1[ ]*longitude:1}`), log.ContextMap()["rpc.request.content"])
		}
		extraOpts := fx.Provide(
			func() []Option {
				payloadFilter := func(_ *otelgrpc.InterceptorInfo) bool {
					return true
				}
				return []Option{WithPayloadFilter(payloadFilter)}
			},
			fx.Annotate(
				func(logger *zap.Logger, opts ...Option) *fxgrpc.StreamServerInterceptor {
					return &fxgrpc.StreamServerInterceptor{Weight: 42, Interceptor: NewLoggingStreamServerInterceptor(logger, opts...)}
				},
				fx.ResultTags(`group:"stream_server_interceptor"`),
			),
		)
		withTestSystem(t, run, extraOpts)
	})

	t.Run("Should log correct payload with clientside stream", func(t *testing.T) {
		run := func(client pb.RouteGuideClient, logs *observer.ObservedLogs) {
			stream, err := client.RecordRoute(context.Background())
			require.NoError(t, err)

			require.NoError(t, stream.Send(&pb.Point{Latitude: 12345, Longitude: 12345}))
			require.NoError(t, stream.Send(&pb.Point{Latitude: 42, Longitude: 42}))

			_, err = stream.CloseAndRecv()
			require.NoError(t, err)

			require.Equal(t, 1, logs.Len())
			log := logs.AllUntimed()[0]
			require.Equal(t, zapcore.DebugLevel, log.Level)
			require.Equal(t, "finished call", log.Message)
			require.Contains(t, log.ContextMap(), "rpc.request.content")
			t.Logf("%T", log.ContextMap()["rpc.request.content"])
			// Using a regex because zap.Any will use the String() implementation of the proto field, which is not stable
			require.Regexp(t, regexp.MustCompile(`latitude:12345[ ]*longitude:12345`), log.ContextMap()["rpc.request.content"])
		}
		extraOpts := fx.Provide(
			func() []Option {
				payloadFilter := func(_ *otelgrpc.InterceptorInfo) bool {
					return true
				}
				return []Option{WithPayloadFilter(payloadFilter)}
			},
			fx.Annotate(
				func(logger *zap.Logger, opts ...Option) *fxgrpc.StreamClientInterceptor {
					return &fxgrpc.StreamClientInterceptor{Weight: 42, Interceptor: NewLoggingStreamClientInterceptor(logger, opts...)}
				},
				fx.ResultTags(`group:"stream_client_interceptor"`),
			),
		)
		withTestSystem(t, run, extraOpts)
	})

	// TODO: test bidirectional stream
	// TODO: test more error cases for logging with streams

	t.Run("Should enrich logger with extraFieldsFunc", func(t *testing.T) {
		run := func(client pb.RouteGuideClient, logs *observer.ObservedLogs) {
			_, err := client.GetFeature(context.Background(), &pb.Point{})
			require.Error(t, err)
			require.Equal(t, codes.Unimplemented, status.Code(err))

			require.Equal(t, 1, logs.Len())
			log := logs.AllUntimed()[0]
			require.Equal(t, zapcore.ErrorLevel, log.Level)
			require.Equal(t, "finished call", log.Message)
			require.Contains(t, log.ContextMap(), "enriched")
			require.True(t, log.ContextMap()["enriched"].(bool))
		}
		extraOpts := fx.Provide(
			func() []Option {
				extraFieldsFunc := func(logger *zap.Logger, _ *otelgrpc.InterceptorInfo, payload any) *zap.Logger {
					return logger.With(zap.Bool("enriched", true))
				}
				return []Option{WithExtraFieldsFunc(extraFieldsFunc)}
			},
			fx.Annotate(
				func(logger *zap.Logger, opts ...Option) *fxgrpc.UnaryServerInterceptor {
					return &fxgrpc.UnaryServerInterceptor{Weight: 42, Interceptor: NewLoggingUnaryServerInterceptor(logger, opts...)}
				},
				fx.ResultTags(`group:"unary_server_interceptor"`),
			),
		)
		withTestSystem(t, run, extraOpts)
	})
}
