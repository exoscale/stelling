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

// NewModule adds pprof support to the system
// Depending on the config it will either spawn a dedicated pprof server
// or directly instrument the process and dump results to a directory
func NewModule(conf PprofConfig) fx.Option {
	if conf.PprofConfig().GenerateFiles != "" {
		return fx.Module(
			"pprof",
			fx.Supply(fx.Annotate(conf, fx.As(new(PprofConfig))), fx.Private),
			fx.Invoke(InvokeRuntimePprof),
		)
	}

	if conf.PprofConfig().Enabled {
		return fx.Module(
			"pprof",
			fx.Supply(fx.Annotate(conf, fx.As(new(PprofConfig))), fx.Private),
			fxhttp.NewModule(&conf.PprofConfig().Server, fxhttp.WithServerModuleName("pprof")),
			fx.Invoke(
				fx.Annotate(
					InitPprofProfiler,
					fx.ParamTags(`name:"pprof"`),
				),
				fx.Annotate(
					fxhttp.StartHttpServer,
					fx.ParamTags("", `name:"pprof"`, ""),
				),
			),
		)
	}

	return fx.Options()
}

type PprofConfig interface {
	PprofConfig() *Pprof
}

type Pprof struct {
	// GenerateFiles generates CPU/Mem pprof file for non deamon process
	GenerateFiles string `validate:"excluded_with=Enabled,omitempty,dir"`
	// Enabled controls the embedded pprof server
	Enabled bool

	Server fxhttp.Server
}

func (p *Pprof) ApplyDefaults() {
	p.Server.Address = ":9092"
}

func (p *Pprof) PprofConfig() *Pprof {
	return p
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
	cpu, err := os.Create(filepath.Join(conf.PprofConfig().GenerateFiles, "pprof.cpu"))
	if err != nil {
		return err
	}
	mem, err := os.Create(filepath.Join(conf.PprofConfig().GenerateFiles, "pprof.mem"))
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
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	server.Handler = mux
}
