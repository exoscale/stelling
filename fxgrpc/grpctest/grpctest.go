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
	UnaryServerInterceptors  []grpc.UnaryServerInterceptor  `group:"unary_server_interceptor"`
	StreamServerInterceptors []grpc.StreamServerInterceptor `group:"stream_server_interceptor"`
	UnaryClientInterceptors  []grpc.UnaryClientInterceptor  `group:"unary_client_interceptor"`
	StreamClientInterceptors []grpc.StreamClientInterceptor `group:"stream_client_interceptor"`
}

func NewGrpc(p GrpcParams) (*grpc.Server, grpc.ClientConnInterface) {
	lis := bufconn.Listen(1024 * 1024)

	bufDialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}
	conn := fxgrpc.NewLazyGrpcClientConn(
		"buffcon",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(p.UnaryClientInterceptors...),
		grpc.WithChainStreamInterceptor(p.StreamClientInterceptors...),
	)

	// Handle server middleware
	unary := []grpc.UnaryServerInterceptor{}
	for i := range p.UnaryServerInterceptors {
		if p.UnaryServerInterceptors[i] != nil {
			unary = append(unary, p.UnaryServerInterceptors[i])
		}
	}
	stream := []grpc.StreamServerInterceptor{}
	for i := range p.StreamServerInterceptors {
		if p.StreamServerInterceptors[i] != nil {
			stream = append(stream, p.StreamServerInterceptors[i])
		}
	}
	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(unary...),
		grpc.ChainStreamInterceptor(stream...),
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

	p.Lc.Append(fx.Hook{
		OnStart: func(c context.Context) error {
			return conn.Start(c)
		},
		OnStop: func(c context.Context) error {
			return conn.Stop(c)
		},
	})

	return s, conn
}
