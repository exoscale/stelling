// package fxhttp provides a convenient way to create well behaved http servers
package fxhttp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	reloader "github.com/exoscale/stelling/fxcert-reloader"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Module = fx.Module(
	"http",
	fx.Provide(
		GetCertReloaderConfig,
		fx.Annotate(reloader.ProvideCertReloader, fx.ParamTags(``, `optional:"true"`, ``)),
		fx.Annotate(NewHTTPServer, fx.ParamTags(``, ``, `optional:"true"`)),
	),
)

func NewNamedModule(name string) fx.Option {
	nameTag := fmt.Sprintf("name:\"%s\"", name)
	optNameTag := fmt.Sprintf("%s, optional:\"true\"", nameTag)
	moduleName := fmt.Sprintf("%s-http-server", name)
	return fx.Options(
		fx.Module(
			moduleName,
			fx.Provide(
				fx.Annotate(
					GetCertReloaderConfig,
					fx.ParamTags(optNameTag),
					fx.ResultTags(nameTag),
				),
				fx.Annotate(
					reloader.ProvideCertReloader,
					fx.ParamTags(``, optNameTag, ``),
					fx.ResultTags(nameTag),
				),
				fx.Annotate(
					NewHTTPServer,
					fx.ParamTags(``, optNameTag, optNameTag),
					fx.ResultTags(nameTag),
				),
			),
		),
		// We're not putting this in the module, so that the module which
		// embeds this can chose when the http server should start
		fx.Invoke(
			fx.Annotate(
				StartHttpServer,
				fx.ParamTags(``, optNameTag, ``, nameTag),
			),
		),
	)
}

type ServerConfig interface {
	GetServer() *Server
}

type Server struct {
	// Port is the port the http server will bind to
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

func GetCertReloaderConfig(conf ServerConfig) *reloader.CertReloaderConfig {
	if conf == nil || !conf.GetServer().TLS {
		return nil
	}
	return &reloader.CertReloaderConfig{
		CertFile:       conf.GetServer().CertFile,
		KeyFile:        conf.GetServer().KeyFile,
		ReloadInterval: 10 * time.Second,
	}
}

func NewHTTPServer(lc fx.Lifecycle, conf ServerConfig, r *reloader.CertReloader) (*http.Server, error) {
	if conf == nil {
		return nil, nil
	}
	server := &http.Server{
		Addr: fmt.Sprintf(":%d", conf.GetServer().Port),
	}

	if conf.GetServer().TLS {
		tlsConf, err := reloader.MakeServerTLS(r, conf.GetServer().ClientCAFile)
		if err != nil {
			return nil, err
		}
		server.TLSConfig = tlsConf
	}

	return server, nil
}

func StartHttpServer(lc fx.Lifecycle, server *http.Server, logger *zap.Logger, conf ServerConfig) {
	if server == nil {
		return
	}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting http server", zap.Int("port", conf.GetServer().Port))
			if conf.GetServer().TLS {
				go func() {
					if err := server.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
						logger.Fatal("Error while serving http", zap.Error(err))
					} else {
						logger.Info("Done serving http")
					}
				}()
			} else {
				go func() {
					if err := server.ListenAndServe(); err != http.ErrServerClosed {
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
