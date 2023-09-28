package fxgrpc

import (
	"context"
	"net"
	"time"

	reloader "github.com/exoscale/stelling/fxcert-reloader"
	fxhttp "github.com/exoscale/stelling/fxhttp"
	zapgrpc "github.com/exoscale/stelling/fxlogging/grpc"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
)

type Config interface {
	GrpcServerConfig() *Server
	AsHttpConfig() *fxhttp.Server
}

type Server struct {
	// imported from fxhttp.Server, copy-pasted to prevent the flag-loader to add a new level

	// A systemd socket name. Takes precedence over Address
	// In order to simplify, only systemd-activated socket with names are allowed, even if it is
	// just one socket
	SocketName string
	// Address is the address+port the server will bind to, as passed to net.Listen
	Address string `default:"localhost:8080"`
	// TLS indicates whether the http server exposes with TLS
	TLS bool
	// CertFile is the path to the pem encoded TLS certificate
	CertFile string `validate:"required_if=TLS true,omitempty,file"`
	// KeyFile is the path to the pem encoded private key of the TLS certificate
	KeyFile string `validate:"required_if=TLS true,omitempty,file"`
	// ClientCAFile is the path to a pem encoded CA cert bundle used to validate clients
	ClientCAFile string `validate:"excluded_without=TLS,omitempty,file"`

	// -imported

	// EnableRecvBufferPool enables the use of grpc buffer pooling in the recv loop
	EnableRecvBufferPool bool
}

func (s *Server) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if s == nil {
		return nil
	}

	enc.AddString("socket-name", s.SocketName)
	enc.AddString("address", s.Address)
	enc.AddBool("tls", s.TLS)

	if s.TLS {
		enc.AddString("cert-file", s.CertFile)
		enc.AddString("key-file", s.KeyFile)
		enc.AddString("client-ca-file", s.ClientCAFile)
	}

	enc.AddBool("enable-recv-buffer-pool", s.EnableRecvBufferPool)

	return nil
}

func (s *Server) GrpcServerConfig() *Server {
	return s
}

func (s *Server) AsHttpConfig() *fxhttp.Server {
	return &fxhttp.Server{
		SocketName:   s.SocketName,
		Address:      s.Address,
		TLS:          s.TLS,
		CertFile:     s.CertFile,
		KeyFile:      s.KeyFile,
		ClientCAFile: s.ClientCAFile,
	}
}

func NewServerModule(conf Config) fx.Option {
	opts := fx.Options(
		fx.Supply(fx.Annotate(conf, fx.As(new(Config)))),
		fx.Supply(
			fx.Annotate(
				conf.AsHttpConfig(),
				fx.As(new(fxhttp.ServerConfig)),
				fx.ResultTags(`name:"grpc_server"`),
			),
		),
		fx.Provide(
			NewGrpcServer,
			func(server *grpc.Server) grpc.ServiceRegistrar { return server },
		),
		fx.Provide(
			fx.Annotate(
				fxhttp.NewListener,
				fx.ParamTags(`name:"grpc_server"`),
				fx.ResultTags(`name:"grpc_server"`),
			),
		),
	)
	if conf.GrpcServerConfig().TLS {
		opts = fx.Options(
			opts,
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

func GetCertReloaderConfig(conf Config) *reloader.CertReloaderConfig {
	if !conf.GrpcServerConfig().TLS {
		return nil
	}
	return &reloader.CertReloaderConfig{
		CertFile:       conf.GrpcServerConfig().CertFile,
		KeyFile:        conf.GrpcServerConfig().KeyFile,
		ReloadInterval: 10 * time.Second,
	}
}

type GrpcServerParams struct {
	fx.In

	Conf               Config
	Logger             *zap.Logger
	UnaryInterceptors  []*UnaryServerInterceptor  `group:"unary_server_interceptor"`
	StreamInterceptors []*StreamServerInterceptor `group:"stream_server_interceptor"`
	Reloader           *reloader.CertReloader     `name:"grpc_server" optional:"true"`
	ServerOpts         []grpc.ServerOption        `group:"grpc_server_options"`
}

func NewGrpcServer(p GrpcServerParams) (*grpc.Server, error) {
	opts := []grpc.ServerOption{}
	serverConf := p.Conf.GrpcServerConfig()

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
	unary := []grpc.UnaryServerInterceptor{}
	for _, ix := range SortInterceptors(p.UnaryInterceptors) {
		unary = append(unary, ix.Interceptor)
	}
	stream := []grpc.StreamServerInterceptor{}
	for _, ix := range SortInterceptors(p.StreamInterceptors) {
		stream = append(stream, ix.Interceptor)
	}
	opts = append(opts, grpc.ChainUnaryInterceptor(unary...), grpc.ChainStreamInterceptor(stream...))

	// Add the externally supplied options last: this allows the user to override any options we may have set already
	opts = append(opts, p.ServerOpts...)

	if serverConf.EnableRecvBufferPool {
		opts = append(opts, grpc.RecvBufferPool(grpc.NewSharedBufferPool()))
	}

	// Set our logger as the logger used by the gRPC framework
	grpclog.SetLoggerV2(zapgrpc.NewLogger(p.Logger))

	grpcServer := grpc.NewServer(opts...)

	return grpcServer, nil
}

type GrpcServerStartParams struct {
	fx.In

	Lc     fx.Lifecycle
	Logger *zap.Logger
	Server *grpc.Server
	Conf   Config
	Lis    net.Listener `name:"grpc_server"`
}

// func StartGrpcServer(lc fx.Lifecycle, logger *zap.Logger, server *grpc.Server, conf Config, lis net.Listener) {
func StartGrpcServer(p GrpcServerStartParams) {
	lc := p.Lc
	logger := p.Logger
	server := p.Server
	// conf := p.conf
	lis := p.Lis

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting gRPC server", zap.String("address", lis.Addr().String()))
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
