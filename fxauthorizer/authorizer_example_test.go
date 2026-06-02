package fxauthorizer_test

import (
	"context"
	"fmt"
	"net/http"

	sconfig "github.com/exoscale/stelling/config"
	"github.com/exoscale/stelling/fxauthorizer"
	"github.com/exoscale/stelling/fxgrpc"
	"github.com/exoscale/stelling/fxgrpc/health"
	"github.com/exoscale/stelling/fxhttp"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	pb "google.golang.org/grpc/examples/route_guide/routeguide"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type GrpcConfig struct {
	fxgrpc.Server
	fxgrpc.Client
	fxauthorizer.Authorizer
}

type HttpConfig struct {
	fxhttp.Server
	fxauthorizer.Authorizer
}

type RouteGuideServer struct {
	pb.UnimplementedRouteGuideServer
}

func NewRouteGuideServer() pb.RouteGuideServer {
	return &RouteGuideServer{}
}

func Example_grpc() {
	conf := &GrpcConfig{}
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
			runGrpc,
		),
	)
	if err := fx.ValidateApp(opts); err != nil {
		panic(err)
	}
	fx.New(opts).Run()

	// Output:
	// Endpoint returned status rpc error: code = PermissionDenied desc = policy denied
	// Healthcheck returned status:SERVING
}

func runGrpc(lc fx.Lifecycle, sd fx.Shutdowner, client pb.RouteGuideClient, healthClient healthpb.HealthClient) {
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

func Example_http() {
	conf := &HttpConfig{}
	rule := "request.path == \"/health\""
	args := []string{"authorizer-test", "--authorizer.rule", rule, "--server.address", "localhost:8081"}
	if err := sconfig.Load(conf, args); err != nil {
		panic(err)
	}

	opts := fx.Options(
		// Suppressing fx logs to ensure deterministic output
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		fxhttp.NewModule(conf),
		fxauthorizer.NewModule(conf),
		fx.Provide(
			zap.NewNop,
			newMux,
		),
		fx.Invoke(
			fxhttp.StartHttpServer,
			runHttp,
		),
	)
	if err := fx.ValidateApp(opts); err != nil {
		panic(err)
	}
	fx.New(opts).Run()

	// Output:
	// Request /foobar returned status 403 Forbidden
	// Request /health returned status 200 OK
}

func newMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}

func runHttp(lc fx.Lifecycle, sd fx.Shutdowner) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				defer sd.Shutdown() //nolint:errcheck
				req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://localhost:8081/foobar", nil)
				if err != nil {
					panic(err)
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					panic(fmt.Sprintln("failed to do http request", err))
				}
				defer resp.Body.Close()
				fmt.Println("Request /foobar returned status", resp.Status)

				req2, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://localhost:8081/health", nil)
				if err != nil {
					panic(err)
				}
				resp2, err := http.DefaultClient.Do(req2)
				if err != nil {
					panic(fmt.Sprintln("failed to do http request", err))
				}
				defer resp2.Body.Close()
				fmt.Println("Request /health returned status", resp2.Status)

			}()
			return nil
		},
	})
}
