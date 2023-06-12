// package fxhttp provides a convenient way to create well behaved http servers
package fxhttp

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	activation "github.com/coreos/go-systemd/activation"
	reloader "github.com/exoscale/stelling/fxcert-reloader"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewModule provides a configured *http.Server to the system
// You still have to invoke StartHttpServer to ensure it starts
func NewModule(conf ServerConfig) fx.Option {
	opts := fx.Options(
		fx.Supply(fx.Annotate(conf, fx.As(new(ServerConfig)))),
		fx.Provide(
			fx.Annotate(NewHTTPServer, fx.ParamTags(``, ``, `optional:"true"`)),
		),
		fx.Provide(NewListener),
	)
	if conf.HttpServerConfig().TLS {
		opts = fx.Options(
			opts,
			fx.Provide(
				GetCertReloaderConfig,
				reloader.ProvideCertReloader,
			),
		)
	}
	return fx.Module("http", opts)
}

// NewNamedModule adds a named http server to the system
// It is mostly of use for other stelling components which may need to
// start their own http servers
// The average application should be able to use NewModule instead
func NewNamedModule(name string, conf ServerConfig) fx.Option {
	nameTag := fmt.Sprintf("name:\"%s\"", name)
	optNameTag := fmt.Sprintf("%s optional:\"true\"", nameTag)
	moduleName := fmt.Sprintf("%s-http-server", name)

	opts := fx.Options(
		fx.Supply(fx.Annotate(conf, fx.As(new(ServerConfig)), fx.ResultTags(nameTag))),
		fx.Provide(
			fx.Annotate(
				NewHTTPServer,
				fx.ParamTags(``, nameTag, optNameTag),
				fx.ResultTags(nameTag),
			),
		),
		fx.Provide(
			fx.Annotate(
				NewListener,
				fx.ParamTags(nameTag),
				fx.ResultTags(nameTag),
			),
		),
	)
	if conf.HttpServerConfig().TLS {
		opts = fx.Options(
			opts,
			fx.Provide(
				fx.Annotate(
					GetCertReloaderConfig,
					fx.ParamTags(nameTag),
					fx.ResultTags(nameTag),
				),
				fx.Annotate(
					reloader.ProvideCertReloader,
					fx.ParamTags(``, nameTag, ``),
					fx.ResultTags(nameTag),
				),
			),
		)
	}
	return fx.Options(
		fx.Module(moduleName, opts),
		// We're not putting this in the module, so that the module which
		// embeds this can chose when the http server should start
		fx.Invoke(
			fx.Annotate(
				StartHttpServer,
				fx.ParamTags(``, nameTag, ``, nameTag, nameTag),
			),
		),
	)
}

type ServerConfig interface {
	HttpServerConfig() *Server
}

type Server struct {
	// Address is the address+port the gRPC server will bind to, as passed to net.Listen
	// Takes precedence over Port
	Address string `validate:"required_without=SocketName,excluded_with=SocketName"`
	// A systemd socket name can also be supplied, but it is mutually exclusive with Address
	// In order to simplify, only systemd-activated socket with names are allowed, even if it is
	// just one socket
	SocketName string `validate:"required_without=Address,excluded_with=Address"`
	// Port is the port the http server will bind to
	// Deprecated
	Port int `default:"8080" validate:"port"`
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

	enc.AddInt("port", s.Port)
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

func NewListener(conf ServerConfig) net.Listener {
	socketName := conf.HttpServerConfig().SocketName

	if socketName != "" {
		listeners, err := activation.ListenersWithNames()
		if err != nil {
			log.Panicf("cannot retrieve listeners: %s", err)
		}
		namedListeners := listeners[socketName]
		if len(namedListeners) != 1 {
			log.Panicf("Named listener count for %s is %d, expected 1", socketName, len(namedListeners))
		}
		listener := namedListeners[0]
		return listener
	} else {
		addr := conf.HttpServerConfig().Address

		if addr == "" {
			addr = fmt.Sprintf(":%d", conf.HttpServerConfig().Port)
		}
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatal("listen error:", err)
		}
		return listener
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

func StartHttpServer(lc fx.Lifecycle, server *http.Server, logger *zap.Logger, conf ServerConfig, lis net.Listener) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting http server", zap.String("address", lis.Addr().String()))
			if conf.HttpServerConfig().TLS {
				go func() {
					if err := server.ServeTLS(lis, "", ""); err != http.ErrServerClosed {
						logger.Fatal("Error while serving http", zap.Error(err))
					} else {
						logger.Info("Done serving http")
					}
				}()
			} else {
				go func() {
					if err := server.Serve(lis); err != http.ErrServerClosed {
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
			return server.Shutdown(ctx)
		},
	})
}
