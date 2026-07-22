package grpc

import (
	"github.com/exoscale/stelling/examples/config"
	pb "github.com/exoscale/stelling/examples/proto/exoscale/examples/v1"
	"go.uber.org/zap"
)

type GRPCServer struct {
	pb.UnimplementedGreeterServiceServer

	logger  *zap.Logger
	message string
}

func NewGRPCServer(
	conf *config.Config,
	logger *zap.Logger,
) *GRPCServer {
	return &GRPCServer{
		logger:  logger,
		message: conf.Greeting.Message,
	}
}
