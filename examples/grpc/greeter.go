package grpc

import (
	"context"

	pb "github.com/exoscale/stelling/examples/proto"
)

func (s *GRPCServer) Greeting(ctx context.Context, req *pb.GreetingRequest) (*pb.GreetingResponse, error) {
	return &pb.GreetingResponse{
		Message: s.message,
	}, nil
}
