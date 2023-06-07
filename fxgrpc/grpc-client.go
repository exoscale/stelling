// Package fxgrpc provides a convenient way to create well behaved grpc servers and clients.
package fxgrpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	reloader "github.com/exoscale/stelling/fxcert-reloader"
	zapgrpc "github.com/exoscale/stelling/fxlogging/grpc"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/grpclog"
)

// TODO: refactor constructors in terms of DialOptions
// This should also make it easier to use outside of an fx system
// Or use fx to manage the tls and middleware, but create clients ad hoc

// NewClientModule Provides a grpc client
func NewClientModule(conf ClientConfig) fx.Option {
	return fx.Module(
		"grpc-client",
		fx.Supply(fx.Annotate(conf, fx.As(new(ClientConfig)))),
		fx.Provide(ProvideGrpcClient),
	)
}

// NewNamedClientModule Provides a named grpc client
func NewNamedClientModule(name string, conf ClientConfig) fx.Option {
	nameTag := fmt.Sprintf("name:\"%s\"", name)

	return fx.Module(
		name,
		fx.Provide(
			func() ClientConfig { return conf },
			fx.Private,
		),
		fx.Provide(
			fx.Annotate(ProvideGrpcClient, fx.ResultTags(nameTag)),
		),
	)
}

// LazyGrpcClientConn is GrpcClientConn that defers initialization of the connection until Start is called
type LazyGrpcClientConn struct {
	conn   *grpc.ClientConn
	target string
	opts   []grpc.DialOption
}

func NewLazyGrpcClientConn(target string, opts ...grpc.DialOption) *LazyGrpcClientConn {
	return &LazyGrpcClientConn{
		target: target,
		opts:   opts,
	}
}

func (c *LazyGrpcClientConn) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	if c.conn == nil {
		return errors.New("LazyGrpcClientConn has not been started yet")
	}
	return c.conn.Invoke(ctx, method, args, reply, opts...)
}

func (c *LazyGrpcClientConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	if c.conn == nil {
		return nil, errors.New("LazyGrpcClientConn has not been started yet")
	}
	return c.conn.NewStream(ctx, desc, method, opts...)
}

// Start initializes the grpc TCP connection
func (c *LazyGrpcClientConn) Start(ctx context.Context) error {
	conn, err := grpc.DialContext(ctx, c.target, c.opts...)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

// Stop closes the grpc TCP connection
func (c *LazyGrpcClientConn) Stop(ctx context.Context) error {
	if c.conn == nil {
		return errors.New("LazyGrpcClientConn has not been started yet")
	}
	return c.conn.Close()
}

type ClientConfig interface {
	GrpcClientConfig() *Client
}

type Client struct {
	// InsecureConnection indicates whether TLS needs to be disabled when connecting to the grpc server
	InsecureConnection bool
	// CertFile is the path to the pem encoded TLS certificate
	CertFile string `validate:"omitempty,file"`
	// KeyFile is the path to the pem encoded private key of the TLS certificate
	KeyFile string `validate:"required_with=CertFile,omitempty,file"`
	// RootCAFile is the  path to a pem encoded CA bundle used to validate server connections
	RootCAFile string `validate:"omitempty,file"`
	// Endpoint is IP or hostname or scheme for the target gRPC server
	Endpoint string `validate:"required"`
}

func (c *Client) GrpcClientConfig() *Client {
	return c
}

func (c *Client) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if c == nil {
		return nil
	}

	enc.AddString("endpoint", c.Endpoint)
	enc.AddBool("insecure-connection", c.InsecureConnection)
	if !c.InsecureConnection {
		enc.AddString("cert-file", c.CertFile)
		enc.AddString("key-file", c.KeyFile)
		enc.AddString("root-ca-file", c.RootCAFile)
	}

	return nil
}

type GrpcClientParams struct {
	fx.In

	Lc                 fx.Lifecycle
	Conf               ClientConfig
	Logger             *zap.Logger
	UnaryInterceptors  []*UnaryClientInterceptor  `group:"unary_client_interceptor"`
	StreamInterceptors []*StreamClientInterceptor `group:"stream_client_interceptor"`
	ClientOpts         []grpc.DialOption          `group:"grpc_client_options"`
}

func MakeClientTLS(c ClientConfig, logger *zap.Logger) (credentials.TransportCredentials, *reloader.CertReloader, error) {
	conf := c.GrpcClientConfig()
	if conf.RootCAFile != "" && conf.CertFile == "" {
		creds, err := credentials.NewClientTLSFromFile(conf.RootCAFile, "")
		return creds, nil, err
	}

	if conf.CertFile != "" {
		// We won't bother using an fx component for the cert reloading.
		// We may have multiple grpc-clients per application and each one
		// of them may be using different certs
		// Expressing that we may have different certs is hard enough for a server
		// (where there can be only one); it's impossible for a client right now
		// We'll just create the reloader in line and register the hooks directly
		r, err := reloader.NewCertReloader(&reloader.CertReloaderConfig{
			CertFile:       conf.CertFile,
			KeyFile:        conf.KeyFile,
			ReloadInterval: 10 * time.Second,
		}, logger)
		if err != nil {
			return nil, nil, err
		}

		tlsConf := &tls.Config{
			GetClientCertificate: r.GetClientCertificate,
		}

		if conf.RootCAFile != "" {
			certPool, err := x509.SystemCertPool()
			if err != nil {
				return nil, nil, err
			}
			ca, err := os.ReadFile(conf.RootCAFile)
			if err != nil {
				return nil, nil, err
			}
			if ok := certPool.AppendCertsFromPEM(ca); !ok {
				return nil, nil, fmt.Errorf("Failed to parse RootCAFile: %s", conf.RootCAFile)
			}
			tlsConf.RootCAs = certPool
		}

		return credentials.NewTLS(tlsConf), r, nil
	}
	return nil, nil, nil
}

func getDialOpts(conf *Client, logger *zap.Logger, ui []grpc.UnaryClientInterceptor, si []grpc.StreamClientInterceptor) ([]grpc.DialOption, *reloader.CertReloader, error) {
	opts := []grpc.DialOption{}
	var creloader *reloader.CertReloader

	if conf.InsecureConnection {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		// We're assuming this is called for a short-lived grpc client
		// The reloader eagerly loads the cert, which is all we want
		// We can ignore it for the remainer
		creds, r, err := MakeClientTLS(conf, logger)
		if err != nil {
			return nil, nil, err
		}
		// TLS is default, but we may not need any clients or ca certs
		if creds != nil {
			opts = append(opts, grpc.WithTransportCredentials(creds))
		}
		creloader = r
	}

	// Handle client middleware
	unary := []grpc.UnaryClientInterceptor{}
	for i := range ui {
		if ui[i] != nil {
			unary = append(unary, ui[i])
		}
	}
	stream := []grpc.StreamClientInterceptor{}
	for i := range si {
		if si[i] != nil {
			stream = append(stream, si[i])
		}
	}
	opts = append(
		opts,
		grpc.WithChainUnaryInterceptor(unary...),
		grpc.WithChainStreamInterceptor(stream...),
	)

	// TODO: move this side effect out into the calling functions?
	grpclog.SetLoggerV2(zapgrpc.NewLogger(logger))

	return opts, creloader, nil
}

// NewGrpcClient returns a grpc client connection that is configured with the same conventions as the fx module
// It is intended to be used for dynamically created, short lived, clients where using fx causes more troubles than benefits
// Because the client is assumed to be short lived, it will not reload TLS certificates
func NewGrpcClient(conf ClientConfig, logger *zap.Logger, ui []*UnaryClientInterceptor, si []*StreamClientInterceptor, dOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	clientConf := conf.GrpcClientConfig()

	unaryIx := make([]grpc.UnaryClientInterceptor, 0, len(ui))
	for _, ix := range SortInterceptors(ui) {
		unaryIx = append(unaryIx, ix.Interceptor)
	}
	streamIx := make([]grpc.StreamClientInterceptor, 0, len(si))
	for _, ix := range SortInterceptors(si) {
		streamIx = append(streamIx, ix.Interceptor)
	}

	opts, _, err := getDialOpts(clientConf, logger, unaryIx, streamIx)
	if err != nil {
		return nil, err
	}

	// Add the externally supplied options last: this allows the user to override any options we may have set already
	opts = append(opts, dOpts...)

	return grpc.Dial(clientConf.Endpoint, opts...)
}

func ProvideGrpcClient(p GrpcClientParams) (grpc.ClientConnInterface, error) {
	clientConf := p.Conf.GrpcClientConfig()

	unaryIx := make([]grpc.UnaryClientInterceptor, 0, len(p.UnaryInterceptors))
	for _, ix := range SortInterceptors(p.UnaryInterceptors) {
		unaryIx = append(unaryIx, ix.Interceptor)
	}
	streamIx := make([]grpc.StreamClientInterceptor, 0, len(p.StreamInterceptors))
	for _, ix := range SortInterceptors(p.StreamInterceptors) {
		streamIx = append(streamIx, ix.Interceptor)
	}
	opts, r, err := getDialOpts(clientConf, p.Logger, unaryIx, streamIx)
	if err != nil {
		return nil, err
	}

	// Add the externally supplied options last: this allows the user to override any options we may have set already
	opts = append(opts, p.ClientOpts...)

	if r != nil {
		p.Lc.Append(fx.Hook{OnStart: r.Start, OnStop: r.Stop})
	}

	conn := NewLazyGrpcClientConn(clientConf.Endpoint, opts...)

	p.Lc.Append(fx.Hook{
		OnStart: conn.Start,
		OnStop:  conn.Stop,
	})

	return conn, nil
}
