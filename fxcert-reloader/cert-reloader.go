package fxcert_reloader

import (
	"context"
	"crypto/tls"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type CertReloaderConfig struct {
	// CertFile is the path to a pem encoded certificate
	CertFile string
	// KeyFile is the path to a pem encoded private key
	KeyFile string
	// The time in which events are buffered up before a reload is attempted
	ReloadInterval time.Duration
}

func (c *CertReloaderConfig) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if c == nil {
		return nil
	}

	enc.AddString("cert-file", c.CertFile)
	enc.AddString("key-file", c.KeyFile)

	return nil
}

// CertReloader watches and reloads a TLS keypair on disk
// Watching for changes must be explicitly started and stopped
// The GetCertificate() method can be used in a tls.Config
type CertReloader struct {
	sync.RWMutex
	cert    *tls.Certificate
	conf    *CertReloaderConfig
	logger  *zap.Logger
	watcher *fsnotify.Watcher
	ticker  *time.Ticker
	wg      sync.WaitGroup
}

// GetCertificate returns the currently loaded keypair
// It is meant to be passed into a tls.Config
// If reloading fails, this method will return the last valid keypair
func (c *CertReloader) GetCertificate() (*tls.Certificate, error) {
	// Naively return our cert
	// Maybe we can try to load if cert is nil
	c.RLock()
	defer c.RUnlock()
	return c.cert, nil
}

// Start spawns a go routine that watches for changes on the KeyPair
func (c *CertReloader) Start(ctx context.Context) error {
	c.logger.Info("Starting watcher")
	// Watching files is extremely hard to get right (surprising, I know)
	// We'll try to annotate the code as best as possible

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	// Because inotify works on inodes, atomic updates (touch + mv) can
	// cause the file watcher to get lost, because the inode changes
	// We will therefore watch the parent directory
	certFileDir := filepath.Dir(c.conf.CertFile)
	if err := watcher.Add(certFileDir); err != nil {
		return err
	}
	// Only watch key directory if it's different
	keyFileDir := filepath.Dir(c.conf.KeyFile)
	if keyFileDir != certFileDir {
		if err := watcher.Add(keyFileDir); err != nil {
			return err
		}
	}
	c.watcher = watcher

	// In order to rate limit a bit and try to prevent reading half written files,
	// we will use a 'dirty' flag to track changes and then use a timer to reload
	// periodically if the certs are 'dirty'
	c.ticker = time.NewTicker(c.conf.ReloadInterval)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		reload := false
		_, certFileName := filepath.Split(c.conf.CertFile)
		_, keyFileName := filepath.Split(c.conf.KeyFile)
		for {
			select {
			case ev, ok := <-c.watcher.Events:
				if !ok {
					// Channel is closed, we can stop the processing here
					c.logger.Info("File watcher channel closed")
					return
				}
				// Because we watch the parent directory, we should only reload if
				// the affected file matches the cert file names
				// We are optimising here for the common case where both cert and key
				// live in the same directory
				// In case they live in a different directory we might reload too often
				// if the same file name lives in both directories, but that should be
				// fine because loading the certs is idempotent
				_, f := filepath.Split(ev.Name)
				if f == certFileName || f == keyFileName {
					c.logger.Info("Certificate was updated. Scheduling update.", zap.Any("event", ev))
					// We don't care about the exact number of events, just that 1 has
					// come in since the last tick
					reload = true
				} else {
					c.logger.Debug("Event for untracked file. Ignoring event.")
				}
			case err, ok := <-c.watcher.Errors:
				if !ok {
					// Channel is closed, we can stop the processing here
					c.logger.Info("File watcher error channel closed")
					return
				}
				// We can't really act on the error here
				// Logging so we can alert on this
				// TODO: expose a count of this as metric?
				c.logger.Error("Error watching for cert changes", zap.Error(err))
			case _, ok := <-c.ticker.C:
				if !ok {
					// Channel is closed, we can stop the processing here
					c.logger.Info("File watcher ticker channel closed")
					return
				}
				if reload {
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
						reload = false
					}
				}
			}
		}
	}()

	return nil
}

// Stop ends the file watcher and cleans up any resources
func (c *CertReloader) Stop(ctx context.Context) error {
	c.logger.Info("Stopping watcher")
	c.ticker.Stop()
	if err := c.watcher.Close(); err != nil {
		return err
	}
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
