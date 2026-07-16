package grpc

import (
	"github.com/exoscale/stelling/examples/config"
	pb "github.com/exoscale/stelling/examples/proto"
	"go.uber.org/zap"
)

type GRPCServer struct {
	pb.UnimplementedGreeterServer

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
