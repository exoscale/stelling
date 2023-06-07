package fxgrpc

import (
	"context"
	"fmt"
	"net"
	"time"

	reloader "github.com/exoscale/stelling/fxcert-reloader"
	fxhttp "github.com/exoscale/stelling/fxhttp"
	zapgrpc "github.com/exoscale/stelling/fxlogging/grpc"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
)

func NewServerModule(conf fxhttp.ServerConfig) fx.Option {
	opts := fx.Options(
		fx.Supply(fx.Annotate(conf, fx.As(new(fxhttp.ServerConfig)))),
		fx.Provide(
			NewGrpcServer,
			func(server *grpc.Server) grpc.ServiceRegistrar { return server },
		),
	)
	if conf.HttpServerConfig().TLS {
		opts = fx.Options(
			fx.Provide(
				fx.Annotate(
					GetCertReloaderConfig,
					fx.ResultTags(`name:"grpc_server"`),
				),
				fx.Annotate(
					reloader.ProvideCertReloader,
					fx.ParamTags(``, `name:"grpc_server"`, ``),
					fx.ResultTags(`name:"grpc_server"`),
				),
			),
		)
	}
	return fx.Module(
		"grpc-server",
		opts,
	)
}

func GetCertReloaderConfig(conf fxhttp.ServerConfig) *reloader.CertReloaderConfig {
	if !conf.HttpServerConfig().TLS {
		return nil
	}
	return &reloader.CertReloaderConfig{
		CertFile:       conf.HttpServerConfig().CertFile,
		KeyFile:        conf.HttpServerConfig().KeyFile,
		ReloadInterval: 10 * time.Second,
	}
}

type GrpcServerParams struct {
	fx.In

	Conf               fxhttp.ServerConfig
	Logger             *zap.Logger
	UnaryInterceptors  []*UnaryServerInterceptor  `group:"unary_server_interceptor"`
	StreamInterceptors []*StreamServerInterceptor `group:"stream_server_interceptor"`
	Reloader           *reloader.CertReloader     `name:"grpc_server" optional:"true"`
	ServerOpts         []grpc.ServerOption        `group:"grpc_server_options"`
}

func NewGrpcServer(p GrpcServerParams) (*grpc.Server, error) {
	opts := []grpc.ServerOption{}
	serverConf := p.Conf.HttpServerConfig()

	// Handle server TLS
	if serverConf.TLS {
		// Due to GetCertReloaderConfig we know we have a reloader here
		creds, err := reloader.MakeServerTLS(p.Reloader, serverConf.ClientCAFile)
		if err != nil {
			return nil, err
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(creds)))
	}

	// Handle server middleware
	SortInterceptors(p.UnaryInterceptors)
	unary := []grpc.UnaryServerInterceptor{}
	for i := range p.UnaryInterceptors {
		unary = append(unary, p.UnaryInterceptors[i].Interceptor)
	}
	SortInterceptors(p.StreamInterceptors)
	stream := []grpc.StreamServerInterceptor{}
	for i := range p.StreamInterceptors {
		stream = append(stream, p.StreamInterceptors[i].Interceptor)
	}
	opts = append(opts, grpc.ChainUnaryInterceptor(unary...), grpc.ChainStreamInterceptor(stream...))

	// Add the externally supplied options last: this allows the user to override any options we may have set already
	opts = append(opts, p.ServerOpts...)

	// Set our logger as the logger used by the gRPC framework
	grpclog.SetLoggerV2(zapgrpc.NewLogger(p.Logger))

	grpcServer := grpc.NewServer(opts...)

	return grpcServer, nil
}

func StartGrpcServer(lc fx.Lifecycle, logger *zap.Logger, server *grpc.Server, conf fxhttp.ServerConfig) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			addr := conf.HttpServerConfig().Address
			if addr == "" {
				addr = fmt.Sprintf(":%d", conf.HttpServerConfig().Port)
			}
			lis, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}
			logger.Info("Starting gRPC server", zap.Int("port", lis.Addr().(*net.TCPAddr).Port))
			go func() {
				if err := server.Serve(lis); err != nil && err != grpc.ErrServerStopped {
					// If err is grpc.ErrServerStopped, it means that
					// the grpc module was stopped very quickly before
					// this goroutine was scheduled
					logger.Fatal("Error while serving grpc", zap.Error(err))
				} else {
					logger.Info("Done serving grpc")
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping gRPC server")
			server.GracefulStop()
			return nil
		},
	})
}
