package fxhttp

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"

	reloader "github.com/exoscale/stelling/fxcert-reloader"
	"go.uber.org/fx"
	"go.uber.org/zap/zapcore"
)

func NewClientModule(conf ClientConfig) fx.Option {
	opts := fx.Options(
		fx.Supply(
			fx.Annotate(conf, fx.As(new(ClientConfig))),
			fx.Private,
		),
		fx.Provide(ProvideHttpClient, fx.Private),
	)
	if conf.HttpClientConfig().CertFile != "" {
		opts = fx.Options(
			opts,
			fx.Provide(
				conf.HttpClientConfig().CertReloaderConfig,
				reloader.ProvideCertReloader,
				fx.Private,
			),
		)
	}
	// Not an actual module, so we can embed this into another one with a private http client
	return opts
}

// NewNamedClientModule provides a named http client
// This is primarily useful embedded in other modules, to construct things like consul and vault
// clients
func NewNamedClientModule(name string, conf ClientConfig) fx.Option {
	nameTag := fmt.Sprintf("name:\"%s\"", name)

	opts := fx.Options(
		fx.Supply(
			fx.Annotate(conf, fx.As(new(ClientConfig))),
			fx.Private,
		),
		fx.Provide(
			fx.Annotate(ProvideHttpClient, fx.ResultTags(nameTag)),
		),
	)
	if conf.HttpClientConfig().CertFile != "" {
		opts = fx.Options(
			opts,
			fx.Provide(
				conf.HttpClientConfig().CertReloaderConfig,
				reloader.ProvideCertReloader,
				fx.Private,
			),
		)
	}
	return fx.Module(name, opts)
}

type ClientConfig interface {
	HttpClientConfig() *Client
}

type Client struct {
	// InsecureConnection indicates whether http server certificate is validated
	InsecureSkipVerify bool
	// CertFile is the path to the pem encoded TLS certificate
	CertFile string `validate:"omitempty,file"`
	// KeyFile is the path to the pem encoded private key of the TLS certificate
	KeyFile string `validate:"required_with=CertFile,omitempty,file"`
	// RootCAFile is the  path to a pem encoded CA bundle used to validate server connections
	RootCAFile string `validate:"omitempty,file"`
}

func (c *Client) HttpClientConfig() *Client {
	return c
}

func (c *Client) CertReloaderConfig() *reloader.CertReloaderConfig {
	return &reloader.CertReloaderConfig{
		CertFile:       c.CertFile,
		KeyFile:        c.KeyFile,
		ReloadInterval: time.Hour,
	}
}

func (c *Client) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if c == nil {
		return nil
	}

	enc.AddBool("insecure-skip-verify", c.InsecureSkipVerify)
	enc.AddString("cert-file", c.CertFile)
	enc.AddString("key-file", c.KeyFile)
	if !c.InsecureSkipVerify {
		enc.AddString("root-ca-file", c.RootCAFile)
	}

	return nil
}

type ProvideHttpClientParams struct {
	fx.In

	Conf     ClientConfig
	Reloader *reloader.CertReloader `optional:"true"`
}

func ProvideHttpClient(p ProvideHttpClientParams) (*http.Client, error) {
	conf := p.Conf.HttpClientConfig()
	certPool, err := SystemPoolWithCA(conf.RootCAFile)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:            certPool,
			InsecureSkipVerify: conf.InsecureSkipVerify,
		},
	}
	if p.Reloader != nil {
		transport.TLSClientConfig.GetClientCertificate = p.Reloader.GetClientCertificate
	}
	return &http.Client{Transport: transport}, nil
}

// SystemPoolWithCA returns the union of the system CA pool with the CA from the given file
// Returns the system CA pool if the path to the extra CA is the empty string
func SystemPoolWithCA(rootCAFile string) (*x509.CertPool, error) {
	certPool, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	if rootCAFile == "" {
		return certPool, nil
	}
	ca, err := os.ReadFile(rootCAFile)
	if err != nil {
		return nil, err
	}
	if ok := certPool.AppendCertsFromPEM(ca); !ok {
		return nil, fmt.Errorf("failed to parse RootCAFile: %s", rootCAFile)
	}
	return certPool, nil
}
