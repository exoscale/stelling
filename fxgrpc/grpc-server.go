package fxgrpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"time"

	reloader "github.com/exoscale/stelling/fxcert-reloader"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zapgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
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
			func(lc fx.Lifecycle, conf *reloader.CertReloaderConfig, logger *zap.Logger) (*reloader.CertReloader, error) {
				if conf == nil {
					return nil, nil
				}
				return reloader.ProvideCertReloader(lc, conf, logger)
			},
			fx.ParamTags("", `name:"grpc_server" optional:"true"`, ""),
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
	Port int `default:"10000" validate:"port"`
}

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

func makeServerTLS(r *reloader.CertReloader, clientCAFile string) (credentials.TransportCredentials, error) {
	tlsConf := &tls.Config{
		GetCertificate: func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) { return r.GetCertificate() },
	}

	if clientCAFile != "" {
		certPool := x509.NewCertPool()
		ca, err := os.ReadFile(clientCAFile)
		if err != nil {
			return nil, err
		}
		if ok := certPool.AppendCertsFromPEM(ca); !ok {
			return nil, fmt.Errorf("Failed to parse ClientCAFile: %s", clientCAFile)
		}
		tlsConf.ClientAuth = tls.RequireAndVerifyClientCert
		tlsConf.ClientCAs = certPool
	}

	return credentials.NewTLS(tlsConf), nil
}

func NewGrpcServer(p GrpcServerParams) (*grpc.Server, error) {
	opts := make([]grpc.ServerOption, 0, 3)
	serverConf := p.Conf.GetServer()

	// Handle server TLS
	if serverConf.TLS {
		// Due to GetCertReloaderConfig we know we have a reloader here
		creds, err := makeServerTLS(p.Reloader, serverConf.ClientCAFile)
		if err != nil {
			return nil, err
		}
		opts = append(opts, grpc.Creds(creds))
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
			p.Logger.Info("Starting gRPC server", zap.Int("port", serverConf.Port))
			go func() {
				if err := grpcServer.Serve(lis); err != nil {
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
