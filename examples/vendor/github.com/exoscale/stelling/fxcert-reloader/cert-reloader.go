// Package fxcert-reloader provides a way to automatically reload certificates
package fxcert_reloader

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type CertReloaderConfig struct {
	// CertFile is the path to a pem encoded certificate
	CertFile string
	// KeyFile is the path to a pem encoded private key
	KeyFile string
	// The time minimum time between 2 reloads
	ReloadInterval time.Duration `default:"1h"`
}

func (c *CertReloaderConfig) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if c == nil {
		return nil
	}

	enc.AddString("cert-file", c.CertFile)
	enc.AddString("key-file", c.KeyFile)
	enc.AddDuration("reload-interval", c.ReloadInterval)

	return nil
}

// CertReloader periodically reloads a TLS keypair on disk.
// The reloader must be explicitly started and stopped
// The GetCertificate() method can be used in a tls.Config
type CertReloader struct {
	cert   *tls.Certificate
	conf   *CertReloaderConfig
	logger *zap.Logger
	wg     sync.WaitGroup
	cancel context.CancelFunc
	sync.RWMutex
}

// GetCertificate returns the currently loaded keypair
// It is meant to be passed into a tls.Config
// If reloading fails, this method will return the last valid keypair
func (c *CertReloader) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.RLock()
	defer c.RUnlock()
	// Naively return our cert
	// Maybe we can try to load if cert is nil
	return c.cert, nil
}

func (c *CertReloader) GetClientCertificate(_ *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	c.RLock()
	defer c.RUnlock()
	return c.cert, nil
}

// Start spawns a go routine that periodically reloads a KeyPair
func (c *CertReloader) Start(ctx context.Context) error {
	c.logger.Info("Starting watcher")

	progCtx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.wg.Add(1)
	go func() {
		ticker := time.NewTicker(c.conf.ReloadInterval)
		defer ticker.Stop()
		defer c.wg.Done()
		for {
			select {
			case <-progCtx.Done():
				return
			case <-ticker.C:
			}
			c.logger.Info("Reloading certificate")
			cert, err := tls.LoadX509KeyPair(c.conf.CertFile, c.conf.KeyFile)
			if err != nil {
				// We are assuming the error is transient and will try to
				// reload on the next tick
				// TODO: expose a count of this as metric?
				c.logger.Error("Failed to reload certificate", zap.Error(err))
			} else {
				c.Lock()
				c.cert = &cert
				c.Unlock()
			}
		}
	}()

	return nil
}

// Stop ends the file watcher and cleans up any resources
func (c *CertReloader) Stop(ctx context.Context) error {
	c.logger.Info("Stopping watcher")
	c.cancel()
	c.wg.Wait()
	return nil
}

// NewCertReloader returns a CertReloader for a KeyPair
// This function will try to eagerly load the KeyPair and will error out if that fails
func NewCertReloader(conf *CertReloaderConfig, logger *zap.Logger) (*CertReloader, error) {
	logger = logger.With(zap.Object("cert", conf))

	logger.Info("Loading certificate")
	cert, err := tls.LoadX509KeyPair(conf.CertFile, conf.KeyFile)
	if err != nil {
		return nil, err
	}

	return &CertReloader{
		cert:   &cert,
		conf:   conf,
		logger: logger,
	}, nil
}

func ProvideCertReloader(lc fx.Lifecycle, conf *CertReloaderConfig, logger *zap.Logger) (*CertReloader, error) {
	if conf == nil {
		return nil, nil
	}
	reloader, err := NewCertReloader(conf, logger)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: reloader.Start,
		OnStop:  reloader.Stop,
	})

	return reloader, nil
}

// MakeServerTLS produces a *tls.Config using a cert reloader and additional config
// TODO: expose more TLS options?
func MakeServerTLS(r *CertReloader, clientCAFile string) (*tls.Config, error) {
	tlsConf := &tls.Config{
		GetCertificate: r.GetCertificate,
	}

	if clientCAFile != "" {
		certPool := x509.NewCertPool()
		ca, err := os.ReadFile(clientCAFile)
		if err != nil {
			return nil, err
		}
		if ok := certPool.AppendCertsFromPEM(ca); !ok {
			return nil, fmt.Errorf("failed to parse ClientCAFile: %s", clientCAFile)
		}
		tlsConf.ClientAuth = tls.RequireAndVerifyClientCert
		tlsConf.ClientCAs = certPool
	}

	return tlsConf, nil
}
