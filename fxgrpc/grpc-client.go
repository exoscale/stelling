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
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zapgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
)

var ClientModule = fx.Provide(
	NewGrpcClient,
)

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

type GrpcClientConfig interface {
	GetClient() *Client
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
	// Endpoint is IP or hostname of the gRPC server
	Endpoint string `validate:"required,omitempty,hostname_port"`
}

func (c *Client) GetClient() *Client {
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
	Conf               GrpcClientConfig
	Logger             *zap.Logger
	UnaryInterceptors  []grpc.UnaryClientInterceptor  `group:"unary_client_interceptor"`
	StreamInterceptors []grpc.StreamClientInterceptor `group:"stream_client_interceptor"`
}

func makeClientTLS(p *GrpcClientParams) (credentials.TransportCredentials, error) {
	conf := p.Conf.GetClient()
	if conf.RootCAFile != "" && conf.CertFile == "" {
		return credentials.NewClientTLSFromFile(conf.RootCAFile, "")
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
		}, p.Logger)
		if err != nil {
			return nil, err
		}
		p.Lc.Append(fx.Hook{
			OnStart: r.Start,
			OnStop:  r.Stop,
		})

		tlsConf := &tls.Config{
			GetCertificate: r.GetCertificate,
		}

		if conf.RootCAFile != "" {
			certPool, err := x509.SystemCertPool()
			if err != nil {
				return nil, err
			}
			ca, err := os.ReadFile(conf.RootCAFile)
			if err != nil {
				return nil, err
			}
			if ok := certPool.AppendCertsFromPEM(ca); !ok {
				return nil, fmt.Errorf("Failed to parse RootCAFile: %s", conf.RootCAFile)
			}
			tlsConf.RootCAs = certPool
		}

		return credentials.NewTLS(tlsConf), nil
	}
	return nil, nil
}

func NewGrpcClient(p GrpcClientParams) (grpc.ClientConnInterface, error) {
	clientConf := p.Conf.GetClient()
	opts := []grpc.DialOption{}

	if !clientConf.InsecureConnection {
		opts = append(opts, grpc.WithInsecure())
	} else {
		creds, err := makeClientTLS(&p)
		if err != nil {
			return nil, err
		}
		// TLS is default, but we may not need any clients or ca certs
		if creds != nil {
			opts = append(opts, grpc.WithTransportCredentials(creds))
		}
	}

	// Handle client middleware
	unary := []grpc.UnaryClientInterceptor{}
	for i := range p.UnaryInterceptors {
		if p.UnaryInterceptors[i] != nil {
			unary = append(unary, p.UnaryInterceptors[i])
		}
	}
	stream := []grpc.StreamClientInterceptor{}
	for i := range p.StreamInterceptors {
		if p.StreamInterceptors[i] != nil {
			stream = append(stream, p.StreamInterceptors[i])
		}
	}
	opts = append(
		opts,
		grpc.WithChainUnaryInterceptor(unary...),
		grpc.WithChainStreamInterceptor(stream...),
	)

	grpclog.SetLoggerV2(zapgrpc.NewLogger(p.Logger))

	conn := NewLazyGrpcClientConn(clientConf.Endpoint, opts...)

	p.Lc.Append(fx.Hook{
		OnStart: conn.Start,
		OnStop:  conn.Stop,
	})

	return conn, nil
}
