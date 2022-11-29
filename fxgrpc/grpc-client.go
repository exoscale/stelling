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

// Provides a grpc client
var ClientModule = fx.Module(
	"grpc-client",
	fx.Provide(
		ProvideGrpcClient(),
	),
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
	// Endpoint is IP or hostname or scheme for the target gRPC server
	Endpoint string `validate:"required,omitempty"`
	// LoadBalancingPolicy is the policy to use for load balancing, empty is ignored.
	LoadBalancingPolicy string `validate:"omitempty,oneof=pick_first round_robin"`
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

	enc.AddString("load-balancing-policy", c.LoadBalancingPolicy)

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

func MakeClientTLS(c GrpcClientConfig, logger *zap.Logger) (credentials.TransportCredentials, *reloader.CertReloader, error) {
	conf := c.GetClient()
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
			GetClientCertificate: func(cri *tls.CertificateRequestInfo) (*tls.Certificate, error) { return r.GetCertificate() },
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

	switch conf.LoadBalancingPolicy {
	case "": // Do nothing
	case "round_robin":
		opts = append(opts, grpc.WithDefaultServiceConfig(`{"loadBalancingConfig": [{"round_robin":{}}]}`))
	case "pick_first":
		opts = append(opts, grpc.WithDefaultServiceConfig(`{"loadBalancingConfig": [{"pick_first":{}}]}`))
	default:
		return nil, nil, fmt.Errorf("invalid loadbalancing policy %s", conf.LoadBalancingPolicy)
	}

	// TODO: move this side effect out into the calling functions?
	grpclog.SetLoggerV2(zapgrpc.NewLogger(logger))

	return opts, creloader, nil
}

// NewGrpcClient returns a grpc client connection that is configured with the same conventions as the fx module
// It is intended to be used for dynamically created, short lived, clients where using fx causes more troubles than benefits
// Because the client is assumed to be short lived, it will not reload TLS certificates
func NewGrpcClient(conf GrpcClientConfig, logger *zap.Logger, ui []grpc.UnaryClientInterceptor,
	si []grpc.StreamClientInterceptor, otherGrpcOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	clientConf := conf.GetClient()

	opts, _, err := getDialOpts(clientConf, logger, ui, si)
	if err != nil {
		return nil, err
	}
	opts = append(opts, otherGrpcOpts...)

	return grpc.Dial(clientConf.Endpoint, opts...)
}

func ProvideGrpcClient(otherGrpcOpts ...grpc.DialOption) func(p GrpcClientParams) (grpc.ClientConnInterface, error) {
	return func(p GrpcClientParams) (grpc.ClientConnInterface, error) {
		clientConf := p.Conf.GetClient()

		opts, r, err := getDialOpts(clientConf, p.Logger, p.UnaryInterceptors, p.StreamInterceptors)
		if err != nil {
			return nil, err
		}

		if r != nil {
			p.Lc.Append(fx.Hook{OnStart: r.Start, OnStop: r.Stop})
		}
		opts = append(opts, otherGrpcOpts...)
		conn := NewLazyGrpcClientConn(clientConf.Endpoint, opts...)

		p.Lc.Append(fx.Hook{
			OnStart: conn.Start,
			OnStop:  conn.Stop,
		})

		return conn, nil
	}
}
