//package fxpprof provides a convenient way to expose pprof endpoint.
package fxpprof

import (
	"context"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	runtimepprof "runtime/pprof"

	"github.com/exoscale/stelling/fxhttp"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Exposes pprof endpoint
var Module = fx.Module(
	"pprof",
	fx.Provide(
		fx.Annotate(
			NewPprofHttpServer,
			fx.ResultTags(`name:"pprof_server"`),
		),
	),
	fx.Invoke(
		fx.Annotate(
			InitPprofProfiler,
			fx.ParamTags(`name:"pprof_server",optional:"true"`),
		),
		InvokeRuntimePprof,
	),
)

type PprofConfig interface {
	GetPprof() *Pprof
}

type Pprof struct {
	// GenerateFiles generates CPU/Mem pprof file for non deamon process
	GenerateFiles string
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
	// ClientCAFile is the path to a pem encoded CA cert bundle used to validate clients
	ClientCAFile string `validate:"excluded_without=TLS,omitempty,file"`
}

func (p *Pprof) GetPprof() *Pprof {
	return p
}

func (p *Pprof) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if p == nil {
		return nil
	}

	enc.AddString("Generatefiles", p.GenerateFiles)
	enc.AddBool("enabled", p.Enabled)

	if p.Enabled {
		enc.AddInt("port", p.Port)
		enc.AddBool("tls", p.TLS)
		if p.TLS {
			enc.AddString("cert-file", p.CertFile)
			enc.AddString("key-file", p.KeyFile)
			enc.AddString("client-ca-file", p.ClientCAFile)
		}
	}

	return nil
}

func NewPprofHttpServer(lc fx.Lifecycle, conf PprofConfig, logger *zap.Logger) (*http.Server, error) {
	if !conf.GetPprof().Enabled {
		return nil, nil
	}

	if conf.GetPprof().GenerateFiles != "" {
		return nil, nil
	}

	sconf := &fxhttp.Server{
		TLS:          conf.GetPprof().TLS,
		CertFile:     conf.GetPprof().CertFile,
		KeyFile:      conf.GetPprof().KeyFile,
		ClientCAFile: conf.GetPprof().ClientCAFile,
		Port:         conf.GetPprof().Port,
	}
	server, err := fxhttp.NewHTTPServer(lc, sconf, logger)
	if err != nil {
		return nil, err
	}
	return server, nil
}

func InvokeRuntimePprof(lc fx.Lifecycle, conf PprofConfig) error {
	if conf.GetPprof().GenerateFiles != "" {
		cpu, err := os.Create(conf.GetPprof().GenerateFiles + ".pprof.cpu")
		if err != nil {
			return err
		}
		mem, err := os.Create(conf.GetPprof().GenerateFiles + ".pprof.mem")
		if err != nil {
			return err
		}

		lc.Append(fx.Hook{
			OnStart: func(c context.Context) error {
				return runtimepprof.StartCPUProfile(cpu)
			},
			OnStop: func(c context.Context) error {
				defer mem.Close()
				defer cpu.Close()

				runtimepprof.StopCPUProfile()
				runtime.GC() // get up-to-date statistics
				return runtimepprof.WriteHeapProfile(mem)
			},
		})
	}

	return nil
}

func InitPprofProfiler(server *http.Server) {
	if server == nil {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	server.Handler = mux
}
