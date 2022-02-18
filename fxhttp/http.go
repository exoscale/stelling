//package fxhttp provides a convenient way to create well behaved http servers
package fxhttp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"

	reloader "github.com/exoscale/stelling/fxcert-reloader"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Module = fx.Provide(
	NewHTTPServer,
)

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
	if !conf.GetServer().TLS {
		return nil
	}
	return &reloader.CertReloaderConfig{
		CertFile:       conf.GetServer().CertFile,
		KeyFile:        conf.GetServer().KeyFile,
		ReloadInterval: 10 * time.Second,
	}
}

// makeTLS produces a *tls.Config using a cert reloader and additional config
// TODO: expose more TLS options?
// TODO: refactor the grpc server version in terms of this one
func makeTLS(r *reloader.CertReloader, clientCAFile string) (*tls.Config, error) {
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

	return tlsConf, nil
}

func NewHTTPServer(lc fx.Lifecycle, conf ServerConfig, logger *zap.Logger) (*http.Server, error) {
	server := &http.Server{
		Addr: fmt.Sprintf(":%d", conf.GetServer().Port),
	}

	if conf.GetServer().TLS {
		// We will be defining the reloader inline to eliminate code in the modules that embed an http server
		// Otherwise these modules would have to wrap around the CertReloader functions to return instances
		// with different names, which is a huge abstraction leak: the reloader is a dependency of the http server
		// and not the upper module; the upper module shouldn't be aware of it.
		// When fx gains support for named subgraphs, we can revisit this.
		// https://github.com/uber-go/dig/pull/240
		reloadConfig := &reloader.CertReloaderConfig{
			CertFile:       conf.GetServer().CertFile,
			KeyFile:        conf.GetServer().KeyFile,
			ReloadInterval: 10 * time.Second,
		}
		r, err := reloader.NewCertReloader(reloadConfig, logger)
		if err != nil {
			return nil, err
		}

		lc.Append(fx.Hook{
			OnStart: r.Start,
			OnStop:  r.Stop,
		})

		tlsConf, err := makeTLS(r, conf.GetServer().ClientCAFile)
		if err != nil {
			return nil, err
		}
		server.TLSConfig = tlsConf
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

	return server, nil
}
