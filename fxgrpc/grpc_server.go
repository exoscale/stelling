package fxgrpc

import (
	"context"
	"fmt"
	"math"
	"time"

	reloader "github.com/exoscale/stelling/fxcert-reloader"
	fxhttp "github.com/exoscale/stelling/fxhttp"
	zapgrpc "github.com/exoscale/stelling/fxlogging/grpc"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/stats"
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

type serverModuleOpts struct {
	name string
}

type serverModuleOption func(*serverModuleOpts)

// WithServerModuleName will annotate the outputs with the given name
func WithServerModuleName(name string) serverModuleOption {
	return func(o *serverModuleOpts) {
		o.name = name
	}
}

func NewServerModule(conf Config, sOpts ...serverModuleOption) fx.Option {
	modOpts := &serverModuleOpts{}
	for _, o := range sOpts {
		o(modOpts)
	}

	opts := fx.Options(
		fx.Supply(fx.Annotate(conf, fx.As(new(Config))), fx.Private),
		fx.Supply(
			fx.Annotate(
				conf.AsHttpConfig(),
				fx.As(new(fxhttp.ServerConfig)),
				fx.ResultTags(`name:"grpc_server"`),
			),
			fx.Private,
		),
		fx.Provide(
			NewGrpcServer,
			fx.Private,
		),
		fx.Invoke(zapgrpc.SetGrpcLogger),
	)
	if modOpts.name == "" {
		opts = fx.Options(
			opts,
			fx.Provide(
				newServer,
				func(s *server) grpc.ServiceRegistrar { return s.server },
				func(s *server) reflection.ServiceInfoProvider { return s.server },
			),
			fx.Invoke(
				func(s *server) { reflection.Register(s.server) },
			),
		)
	} else {
		nameTag := fmt.Sprintf("name:\"%s\"", modOpts.name)
		opts = fx.Options(
			opts,
			fx.Provide(
				fx.Annotate(
					newServer,
					fx.ResultTags(nameTag),
				),
				fx.Annotate(
					func(s *server) grpc.ServiceRegistrar { return s.server },
					fx.ParamTags(nameTag),
					fx.ResultTags(nameTag),
				),
				fx.Annotate(
					func(s *server) reflection.ServiceInfoProvider { return s.server },
					fx.ParamTags(nameTag),
					fx.ResultTags(nameTag),
				),
			),
			fx.Invoke(
				fx.Annotate(
					func(s *server) { reflection.Register(s.server) },
					fx.ParamTags(nameTag),
				),
			),
		)
	}
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
				fx.Private,
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
		ReloadInterval: 1 * time.Hour,
	}
}

// server is a tuple of grpc.Server with its accompanying address or socket name
type server struct {
	server     *grpc.Server
	addr       string
	socketName string
}

func newServer(s *grpc.Server, conf Config) *server {
	return &server{s, conf.AsHttpConfig().Address, conf.AsHttpConfig().SocketName}
}

type GrpcServerParams struct {
	fx.In

	Conf               Config
	StatsHandlers      []stats.Handler            `group:"server_stats_handler"`
	UnaryInterceptors  []*UnaryServerInterceptor  `group:"unary_server_interceptor"`
	StreamInterceptors []*StreamServerInterceptor `group:"stream_server_interceptor"`
	Reloader           *reloader.CertReloader     `name:"grpc_server" optional:"true"`
	ServerOpts         []grpc.ServerOption        `group:"grpc_server_options"`
}

func NewGrpcServer(p GrpcServerParams) (*grpc.Server, error) {
	opts := []grpc.ServerOption{
		// We overwrite the default keepalive parameters, because they interact badly with the keepalive config of the go runtime
		// Go runtime sets the keepalive interval to 15s: https://github.com/golang/go/blob/0d347544cbca0f42b160424f6bc2458ebcc7b3fc/src/net/dial.go#LL17C1-L17C1
		// Grpc sets TCP_USER_TIMEOUT to 20s: https://github.com/grpc/grpc-go/blob/master/internal/transport/defaults.go#L39
		// and https://github.com/grpc/grpc-go/blob/273e03d8d31b433965b06029ee03e26d298bd8c1/internal/transport/http2_server.go#L232-L239
		// The kernel computes the max number of packets that can be missed by dividing these numbers (https://man7.org/linux/man-pages/man7/tcp.7.html)
		// Therefore a single dropped keepalive packet will make the grpc server terminate the TCP connection with rst
		//
		// Grpc introduced the usage of TCP_USER_TIMEOUT because it made memory management of its C++ backend better:
		// https://github.com/grpc/proposal/blob/master/A18-tcp-user-timeout.md
		//
		// Given that we do not use the C++ backend, we do not need this particular optimization:
		// * The standard usage of short TCP keepalive intervals in the go runtime will ensure dead connections are detected quickly
		//   (135s on default Linux systems, compared to multiple hours in the standard case)
		// * Go's GC is capable of cleaning up memory well enough in case TCP connections are killed
		// * Applications for which this is still too long can still set application level deadlines on individual requests
		//
		// We therefore chose to wholesale disable the use of TCP_USER_TIMEOUT in grpc: it makes finding a good enough point in the configuration space too hard
		grpc.KeepaliveParams(keepalive.ServerParameters{
			// See: https://github.com/grpc/grpc-go/blob/8389ddb30539e08e45391650f8c249bd8a57ffc3/internal/transport/http2_server.go#L229-L239
			Time: time.Duration(math.MaxInt64), // Disables TCP_USER_TIMEOUT
		}),
	}
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

	// Stats handlers
	for _, handler := range p.StatsHandlers {
		opts = append(opts, grpc.StatsHandler(handler))
	}

	// Handle server middleware
	opts = append(
		opts,
		UnaryServerInterceptors(p.UnaryInterceptors),
		StreamServerInterceptors(p.StreamInterceptors),
	)

	// Add the externally supplied options last: this allows the user to override any options we may have set already
	opts = append(opts, p.ServerOpts...)

	grpcServer := grpc.NewServer(opts...)

	return grpcServer, nil
}

func StartGrpcServer(lc fx.Lifecycle, logger *zap.Logger, s *server) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting gRPC server", zap.String("address", s.addr))
			lis, err := fxhttp.NewListener(ctx, s.socketName, s.addr)
			if err != nil {
				return err
			}
			go func() {
				if err := s.server.Serve(lis); err != nil && err != grpc.ErrServerStopped {
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
			s.server.GracefulStop()
			return nil
		},
	})
}
