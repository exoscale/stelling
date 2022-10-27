package fxgrpc

import (
	"context"
	"fmt"
	"net"
	"time"

	reloader "github.com/exoscale/stelling/fxcert-reloader"
	zapgrpc "github.com/exoscale/stelling/fxlogging/grpc"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/keepalive"
)

// Provides a grpc server
var ServerModule = fx.Module(
	"grpc-server",
	fx.Provide(
		fx.Annotate(
			GetCertReloaderConfig,
			fx.ResultTags(`name:"grpc_server"`),
		),
		fx.Annotate(
			reloader.ProvideCertReloader,
			fx.ParamTags(``, `name:"grpc_server" optional:"true"`, ``),
			fx.ResultTags(`name:"grpc_server"`),
		),
		NewGrpcServer,
	),
)

type GrpcServerConfig interface {
	GetServer() *Server
}

type Server struct {
	// TLS indicates whether the service exposes a TLS endpoint
	TLS bool
	// CertFile is the path to the pem encoded TLS certificate
	CertFile string `validate:"required_if=TLS true,omitempty,file"`
	// KeyFile is the path to the pem encoded private key of the TLS certificate
	KeyFile string `validate:"required_if=TLS true,omitempty,file"`
	// ClientCAFile is the path to a pem encoded CA cert bundle used to validate clients
	ClientCAFile string `validate:"excluded_without=TLS,omitempty,file"`
	// Port is the port the gRPC server will bind to
	Port int `default:"0" validate:"port"`
	// Keepalive is the configuration of the grpc-keepalive
	Keepalive Keepalive
}

// Struct used to reflect the type
type Keepalive keepalive.ServerParameters

func (s *Server) GetServer() *Server {
	return s
}

func (s *Server) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if s == nil {
		return nil
	}

	enc.AddInt("port", s.Port)
	enc.AddBool("tls", s.TLS)
	if s.TLS {
		enc.AddString("cert-file", s.CertFile)
		enc.AddString("key-file", s.KeyFile)
		enc.AddString("client-ca-file", s.ClientCAFile)
	}

	if err := enc.AddObject("keepalive", &s.Keepalive); err != nil {
		return err
	}

	return nil
}

func (k *Keepalive) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if k == nil {
		return nil
	}

	enc.AddDuration("max-connection-idle", k.MaxConnectionIdle)
	enc.AddDuration("max-connection-age", k.MaxConnectionAge)
	enc.AddDuration("max-connection-age-grace", k.MaxConnectionAgeGrace)
	enc.AddDuration("time", k.Time)
	enc.AddDuration("timeout", k.Timeout)

	return nil
}

func GetCertReloaderConfig(conf GrpcServerConfig) *reloader.CertReloaderConfig {
	if !conf.GetServer().TLS {
		return nil
	}
	return &reloader.CertReloaderConfig{
		CertFile:       conf.GetServer().CertFile,
		KeyFile:        conf.GetServer().KeyFile,
		ReloadInterval: 10 * time.Second,
	}
}

type GrpcServerParams struct {
	fx.In

	Lc                 fx.Lifecycle
	Conf               GrpcServerConfig
	Logger             *zap.Logger
	UnaryInterceptors  []grpc.UnaryServerInterceptor  `group:"unary_server_interceptor"`
	StreamInterceptors []grpc.StreamServerInterceptor `group:"stream_server_interceptor"`
	Reloader           *reloader.CertReloader         `name:"grpc_server" optional:"true"`
}

func NewGrpcServer(p GrpcServerParams) (*grpc.Server, error) {
	opts := []grpc.ServerOption{}
	serverConf := p.Conf.GetServer()

	// Handle keepalive configuration
	opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters(serverConf.Keepalive)))

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
	opts = append(opts, grpc.ChainUnaryInterceptor(unary...), grpc.ChainStreamInterceptor(stream...))

	// Set our logger as the logger used by the gRPC framework
	grpclog.SetLoggerV2(zapgrpc.NewLogger(p.Logger))

	grpcServer := grpc.NewServer(opts...)

	p.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			lis, err := net.Listen("tcp", fmt.Sprintf(":%d", serverConf.Port))
			if err != nil {
				return err
			}
			p.Logger.Info("Starting gRPC server", zap.Int("port", lis.Addr().(*net.TCPAddr).Port))
			go func() {
				if err := grpcServer.Serve(lis); err != nil && err != grpc.ErrServerStopped {
					// If err is grpc.ErrServerStopped, it means that
					// the grpc module was stopped very quickly before
					// this goroutine was scheduled
					p.Logger.Fatal("Error while serving grpc", zap.Error(err))
				} else {
					p.Logger.Info("Done serving grpc")
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			p.Logger.Info("Stopping gRPC server")
			grpcServer.GracefulStop()
			return nil
		},
	})

	return grpcServer, nil
}
