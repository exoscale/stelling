package fxgrpc

import (
	"context"
	"errors"

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
	// TLS indicates whether TLS needs to be used to connect to the grpc server
	TLS bool
	// CertFile is the path to the pem encoded TLS certificate
	CertFile string `validate:"required_if=TLS true,omitempty,file"`
	// KeyFile is the path to the pem encoded private key of the TLS certificate
	KeyFile string `validate:"required_if=TLS true,omitempty,file"`
	// Endpoint is IP or hostname of the gRPC server
	Endpoint string `validate:"required_if=TLS true,omitempty,hostname_port"`
}

func (c *Client) GetClient() *Client {
	return c
}

func (c *Client) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if c == nil {
		return nil
	}

	enc.AddString("endpoint", c.Endpoint)
	enc.AddBool("tls", c.TLS)
	if c.TLS {
		enc.AddString("cert-file", c.CertFile)
		enc.AddString("key-file", c.KeyFile)
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

func NewGrpcClient(p GrpcClientParams) (grpc.ClientConnInterface, error) {
	clientConf := p.Conf.GetClient()
	opts := []grpc.DialOption{}

	if !clientConf.TLS {
		opts = append(opts, grpc.WithInsecure())
	} else {
		creds, err := credentials.NewServerTLSFromFile(clientConf.CertFile, clientConf.KeyFile)
		if err != nil {
			return nil, err
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
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
		OnStart: func(c context.Context) error {
			return conn.Start(c)
		},
		OnStop: func(c context.Context) error {
			return conn.Stop(c)
		},
	})

	return conn, nil
}
