// package fxhttp provides a convenient way to create well behaved http servers
package fxhttp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	reloader "github.com/exoscale/stelling/fxcert-reloader"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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

// server is a tuple of http.Server with its accompanying net.Listener
// It allows us to keep the server and listener constructors private to this module
// While providing a single output of the module that be named, in case we need multiple server instances
type server struct {
	server *http.Server
	lis    net.Listener
}

func newServer(s *http.Server, lis net.Listener) *server {
	return &server{s, lis}
}

// NewModule provides a configured *http.Server to the system
// You still have to invoke StartHttpServer to ensure it starts
func NewModule(conf ServerConfig, sOpts ...serverModuleOption) fx.Option {
	modOpts := &serverModuleOpts{}
	for _, o := range sOpts {
		o(modOpts)
	}

	opts := fx.Options(
		fx.Supply(
			fx.Annotate(conf, fx.As(new(ServerConfig))),
			fx.Private,
		),
		fx.Provide(
			NewListener,
			fx.Private,
		),
	)
	if modOpts.name == "" {
		opts = fx.Options(
			opts,
			fx.Provide(
				fx.Annotate(NewHTTPServer, fx.ParamTags(``, ``, `optional:"true"`)),
				newServer,
			),
		)
	} else {
		nameTag := fmt.Sprintf("name:\"%s\"", modOpts.name)
		opts = fx.Options(
			opts,
			fx.Provide(
				fx.Annotate(
					NewHTTPServer,
					fx.ParamTags(``, ``, `optional:"true"`),
					fx.ResultTags(nameTag),
				),
				fx.Annotate(
					newServer,
					fx.ParamTags(nameTag, ""),
					fx.ResultTags(nameTag),
				),
			),
		)
	}
	if conf.HttpServerConfig().TLS {
		opts = fx.Options(
			opts,
			fx.Provide(
				GetCertReloaderConfig,
				reloader.ProvideCertReloader,
				fx.Private,
			),
		)
	}
	return fx.Module("http", opts)
}

// NewNamedModule adds a named http server to the system
// It is mostly of use for other stelling components which may need to
// start their own http servers
// The average application should be able to use NewModule instead
//
// Deprecated: please use [NewModule] with the [WithServerModuleName] option
// and add `fx.Invoke(fx.Annotate(StartHttpServer), fx.ParamTags("", "name=\"yourname\"", ""))` to the system
func NewNamedModule(name string, conf ServerConfig) fx.Option {
	nameTag := fmt.Sprintf("name:\"%s\"", name)
	return fx.Options(
		NewModule(conf, WithServerModuleName(name)),
		// We're not putting this in the module, so that the module which
		// embeds this can chose when the http server should start
		fx.Invoke(fx.Annotate(StartHttpServer, fx.ParamTags(``, nameTag, ``))),
	)
}

type ServerConfig interface {
	HttpServerConfig() *Server
}

type Server struct {
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

func (s *Server) HttpServerConfig() *Server {
	return s
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

func GetCertReloaderConfig(conf ServerConfig) *reloader.CertReloaderConfig {
	return &reloader.CertReloaderConfig{
		CertFile:       conf.HttpServerConfig().CertFile,
		KeyFile:        conf.HttpServerConfig().KeyFile,
		ReloadInterval: 10 * time.Second,
	}
}

func NewListener(conf ServerConfig) (net.Listener, error) {
	socketName := conf.HttpServerConfig().SocketName

	if socketName != "" {
		return NamedSocketListener(socketName)
	} else {
		return net.Listen("tcp", conf.HttpServerConfig().Address)
	}
}

func NewHTTPServer(lc fx.Lifecycle, conf ServerConfig, r *reloader.CertReloader) (*http.Server, error) {
	server := &http.Server{}

	if conf.HttpServerConfig().TLS {
		tlsConf, err := reloader.MakeServerTLS(r, conf.HttpServerConfig().ClientCAFile)
		if err != nil {
			return nil, err
		}
		server.TLSConfig = tlsConf
	}

	return server, nil
}

func StartHttpServer(lc fx.Lifecycle, s *server, logger *zap.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting http server", zap.String("address", s.lis.Addr().String()))
			if s.server.TLSConfig != nil {
				go func() {
					if err := s.server.ServeTLS(s.lis, "", ""); err != http.ErrServerClosed {
						logger.Fatal("Error while serving http", zap.Error(err))
					} else {
						logger.Info("Done serving http")
					}
				}()
			} else {
				go func() {
					if err := s.server.Serve(s.lis); err != http.ErrServerClosed {
						logger.Fatal("Error while serving http", zap.Error(err))
					} else {
						logger.Info("Done serving http")
					}
				}()
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping http server")
			return s.server.Shutdown(ctx)
		},
	})
}
