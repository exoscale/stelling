package grpctest

import (
	"context"
	"net"

	"github.com/exoscale/stelling/fxgrpc"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
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
	),
)

func NewGrpc(p fxgrpc.GrpcServerParams) (*grpc.Server, grpc.ClientConnInterface) {
	lis := bufconn.Listen(1024 * 1024)

	bufDialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}
	conn := fxgrpc.NewLazyGrpcClientConn(
		"buffcon",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	// Handle server middleware
	unary := []grpc.UnaryServerInterceptor{grpc_ctxtags.UnaryServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor))}
	for i := range p.UnaryInterceptors {
		if p.UnaryInterceptors[i] != nil {
			unary = append(unary, p.UnaryInterceptors[i])
		}
	}
	stream := []grpc.StreamServerInterceptor{grpc_ctxtags.StreamServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor))}
	for i := range p.StreamInterceptors {
		if p.StreamInterceptors[i] != nil {
			stream = append(stream, p.StreamInterceptors[i])
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
