package interceptor

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	peerServiceMDKey = "peer.service"
)

// NewInjectPeerUnaryClientInterceptor produces a UnaryClientInterceptor that sets the
// peer.service on the metadata of the outgoing context
// It can then be logged by the server to identify the service making the request
func NewInjectPeerUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, callopts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, peerServiceMDKey, serviceName())
		return invoker(ctx, method, req, reply, cc, callopts...)
	}
}

// NewInjectPeerStreamClientInterceptor produces a StreamClientInterceptor that sets the
// peer.service on the metadata of the outgoing context
// It can then be logged by the server to identify the service making the request
func NewInjectPeerStreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, callOpts ...grpc.CallOption) (grpc.ClientStream, error) {
		ctx = metadata.AppendToOutgoingContext(ctx, peerServiceMDKey, serviceName())
		return streamer(ctx, desc, cc, method, callOpts...)
	}
}
