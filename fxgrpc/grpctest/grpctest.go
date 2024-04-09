package grpctest

import (
	"context"
	"net"

	"github.com/exoscale/stelling/fxgrpc"
	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// Module provides a grpc Server and ClientConn that use a buffer instead of the actual network to communicate
var Module = fx.Module(
	"grpc-test",
	fx.Provide(
		NewGrpc,
		func(server *grpc.Server) grpc.ServiceRegistrar { return server },
	),
)

type GrpcParams struct {
	fx.In

	Lc                       fx.Lifecycle
	UnaryServerInterceptors  []*fxgrpc.UnaryServerInterceptor  `group:"unary_server_interceptor"`
	StreamServerInterceptors []*fxgrpc.StreamServerInterceptor `group:"stream_server_interceptor"`
	UnaryClientInterceptors  []*fxgrpc.UnaryClientInterceptor  `group:"unary_client_interceptor"`
	StreamClientInterceptors []*fxgrpc.StreamClientInterceptor `group:"stream_client_interceptor"`
}

func NewGrpc(p GrpcParams) (*grpc.Server, grpc.ClientConnInterface, error) {
	lis := bufconn.Listen(1024 * 1024)

	bufDialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}
	conn, err := grpc.NewClient(
		"passthrough://buffcon",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		fxgrpc.WithUnaryClientInterceptors(p.UnaryClientInterceptors),
		fxgrpc.WithStreamClientInterceptors(p.StreamClientInterceptors),
	)
	if err != nil {
		return nil, nil, err
	}

	// Handle server middleware
	s := grpc.NewServer(
		fxgrpc.UnaryServerInterceptors(p.UnaryServerInterceptors),
		fxgrpc.StreamServerInterceptors(p.StreamServerInterceptors),
	)

	p.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			go s.Serve(lis) //nolint: errcheck
			return nil
		},
		OnStop: func(_ context.Context) error {
			s.Stop()
			return nil
		},
	})

	return s, conn, err
}
