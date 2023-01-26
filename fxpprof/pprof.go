// package fxpprof provides a convenient way to expose pprof endpoint.
package fxpprof

import (
	"context"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	runtimepprof "runtime/pprof"

	"github.com/exoscale/stelling/fxhttp"
	"go.uber.org/fx"
	"go.uber.org/zap/zapcore"
)

// Exposes pprof endpoint
var Module = fx.Module(
	"pprof",
	fx.Provide(
		fx.Annotate(
			NewPprofServerConfig,
			fx.ResultTags(`name:"pprof"`),
		),
	),
	fx.Invoke(
		fx.Annotate(
			InitPprofProfiler,
			fx.ParamTags(`name:"pprof",optional:"true"`),
		),
		InvokeRuntimePprof,
	),
	// Specify last so the server starts after we register the handlers
	fxhttp.NewNamedModule("pprof"),
)

type PprofConfig interface {
	GetPprof() *Pprof
}

type Pprof struct {
	// GenerateFiles generates CPU/Mem pprof file for non deamon process
	GenerateFiles string `validate:"excluded_with=Enabled,omitempty,dir"`
	// Enabled controls the embedded pprof server
	Enabled bool

	fxhttp.Server
}

func (p *Pprof) ApplyDefauls() {
	p.Server.Port = 9092
}

func (p *Pprof) GetPprof() *Pprof {
	return p
}

func NewPprofServerConfig(conf PprofConfig) fxhttp.ServerConfig {
	pConf := conf.GetPprof()
	if !pConf.Enabled {
		return nil
	}
	return &pConf.Server
}

func (p *Pprof) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if p == nil {
		return nil
	}

	enc.AddString("generatefiles", p.GenerateFiles)
	enc.AddBool("enabled", p.Enabled)

	if p.Enabled {
		if err := enc.AddObject("server", &p.Server); err != nil {
			return err
		}
	}

	return nil
}

func InvokeRuntimePprof(lc fx.Lifecycle, conf PprofConfig) error {
	if conf.GetPprof().GenerateFiles == "" {
		return nil
	}
	cpu, err := os.Create(filepath.Join(conf.GetPprof().GenerateFiles, "pprof.cpu"))
	if err != nil {
		return err
	}
	mem, err := os.Create(filepath.Join(conf.GetPprof().GenerateFiles, "pprof.mem"))
	if err != nil {
		cpu.Close()
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
