package reflection

import (
	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var Module = fx.Options(
	fx.Invoke(Register),
)

func Register(s *grpc.Server) {
	reflection.Register(s)
}
