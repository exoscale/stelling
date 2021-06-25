package fxpprof

import (
	"context"
	"fmt"
	"net/http"
	"net/http/pprof"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Module = fx.Options(
	fx.Provide(NewPprofProfiler),
	fx.Invoke(InitPprofProfiler),
)

type PprofConfig interface {
	GetPprof() *Pprof
}

type Pprof struct {
	// Enabled controls the embedded pprof server
	Enabled bool
	// Port is the port the Pprof endpoint will bind to
	Port int `default:"9092" validate:"port"`
	// TLS indicates whether the Pprof endpoint exposes with TLS
	TLS bool
	// CertFile is the path to the pem encoded TLS certificate
	CertFile string `validate:"required_if=TLS true,omitempty,file"`
	// KeyFile is the path to the pem encoded private key of the TLS certificate
	KeyFile string `validate:"required_if=TLS true,omitempty,file"`
}

func (p *Pprof) GetPprof() *Pprof {
	return p
}

func (p *Pprof) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if p == nil {
		return nil
	}

	enc.AddBool("enabled", p.Enabled)

	if p.Enabled {
		enc.AddInt("port", p.Port)
		enc.AddBool("tls", p.TLS)
		if p.TLS {
			enc.AddString("cert-file", p.CertFile)
			enc.AddString("key-file", p.KeyFile)
		}
	}

	return nil
}

type PprofProfiler struct{}

func NewPprofProfiler(lc fx.Lifecycle, conf PprofConfig, logger *zap.Logger) *PprofProfiler {
	pConf := conf.GetPprof()

	if !pConf.Enabled {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", pConf.Port),
		Handler: mux,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting pprof server", zap.Int("port", pConf.Port))
			if pConf.TLS {
				go func() {
					if err := server.ListenAndServeTLS(pConf.CertFile, pConf.KeyFile); err != http.ErrServerClosed {
						logger.Fatal("Error while serving pprof", zap.Error(err))
					} else {
						logger.Info("Done serving pprof")
					}
				}()
			} else {
				go func() {
					if err := server.ListenAndServe(); err != http.ErrServerClosed {
						logger.Fatal("Error while serving pprof", zap.Error(err))
					} else {
						logger.Info("Done serving pprof")
					}
				}()
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping pprof server")
			return server.Shutdown(ctx)
		},
	})

	return &PprofProfiler{}
}

type InitPprofProfileParams struct {
	fx.In

	Prof   *PprofProfiler `optional:"true"`
	Logger *zap.Logger
}

func InitPprofProfiler(p InitPprofProfileParams) {
	if p.Prof != nil {
		p.Logger.Info("Enabling pprof profiling")
	}
}
