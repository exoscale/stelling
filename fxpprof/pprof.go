package fxpprof

import (
	"net/http"
	"net/http/pprof"

	"github.com/exoscale/stelling/fxhttp"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Module = fx.Options(
	fx.Provide(
		NewPprofHttpServer,
	),
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

type PprofHttpServerResult struct {
	fx.Out

	Server *http.Server `name:"pprof_server"`
}

func NewPprofHttpServer(lc fx.Lifecycle, conf PprofConfig, logger *zap.Logger) (PprofHttpServerResult, error) {
	sconf := &fxhttp.Server{
		TLS:          conf.GetPprof().TLS,
		CertFile:     conf.GetPprof().CertFile,
		KeyFile:      conf.GetPprof().KeyFile,
		ClientCAFile: conf.GetPprof().ClientCAFile,
		Port:         conf.GetPprof().Port,
	}
	server, err := fxhttp.NewHTTPServer(lc, sconf, logger)
	if err != nil {
		return PprofHttpServerResult{}, err
	}
	return PprofHttpServerResult{
		Server: server,
	}, nil
}

type InitPprofProfileParams struct {
	fx.In

	Server *http.Server `name:"pprof_server" optional:"true"`
}

func InitPprofProfiler(p InitPprofProfileParams) {
	if p.Server == nil {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	p.Server.Handler = mux
}
