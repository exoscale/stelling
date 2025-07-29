package fxgrpc

import (
	"context"
	"errors"
	"fmt"
	"sync"

	fxcert_reloader "github.com/exoscale/stelling/fxcert-reloader"
	"go.uber.org/fx"
	"google.golang.org/grpc"
)

func NewConnManagerModule(conf ConnManagerConfig) fx.Option {
	return fx.Module(
		"grpc-conn-manager",
		fx.Supply(fx.Annotate(conf, fx.As(new(ConnManagerConfig)))),
		fx.Provide(
			fx.Annotate(
				func(conf ConnManagerConfig) ClientConfig {
					return &Client{
						InsecureConnection: conf.ConnManagerConfig().InsecureConnection,
						CertFile:           conf.ConnManagerConfig().CertFile,
						KeyFile:            conf.ConnManagerConfig().KeyFile,
						RootCAFile:         conf.ConnManagerConfig().RootCAFile,
					}
				},
				fx.ResultTags(`name:"grpc_conn_manager"`),
			),
			fx.Annotate(
				MakeClientTLS,
				fx.ParamTags(`name:"grpc_conn_manager"`),
				fx.ResultTags(``, `name:"grpc_conn_manager"`),
			),
			fx.Annotate(
				grpc.WithTransportCredentials,
				fx.ResultTags(`group:"grpc_client_options"`),
			),
			fx.Annotate(
				WithStreamClientInterceptors,
				fx.ParamTags(`group:"stream_client_interceptors"`),
				fx.ResultTags(`group:"grpc_client_options"`),
			),
			fx.Annotate(
				WithUnaryClientInterceptors,
				fx.ParamTags(`group:"unary_client_interceptors"`),
				fx.ResultTags(`group:"grpc_client_options"`),
			),
			fx.Private,
		),
		fx.Provide(
			ProvideConnManager,
		),
	)
}

type ConnManagerConfig interface {
	ConnManagerConfig() *ConnManagerOpts
}

type ConnManagerOpts struct {
	// InsecureConnection indicates whether TLS needs to be disabled when connecting to the grpc server
	InsecureConnection bool
	// CertFile is the path to the pem encoded TLS certificate
	CertFile string `validate:"omitempty,file"`
	// KeyFile is the path to the pem encoded private key of the TLS certificate
	KeyFile string `validate:"required_with=CertFile,omitempty,file"`
	// RootCAFile is the  path to a pem encoded CA bundle used to validate server connections
	RootCAFile string `validate:"omitempty,file"`
}

func (c *ConnManagerOpts) ConnManagerConfig() *ConnManagerOpts {
	return c
}

// ConnManager is a cache of grpc.ClientConn's
// Users of the manager should leave the lifecycle of the
// underlying gRPC connections entirely up to the manager
type ConnManager struct {
	lock sync.RWMutex
	idx  map[string]*grpc.ClientConn
	opts []grpc.DialOption
}

func NewConnManager(opts []grpc.DialOption) *ConnManager {
	return &ConnManager{
		idx:  make(map[string]*grpc.ClientConn),
		opts: opts,
	}
}

func (m *ConnManager) Stop(ctx context.Context) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	var errs error
	for _, conn := range m.idx {
		if err := conn.Close(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

type ConnManagerParams struct {
	fx.In

	Lc                 fx.Lifecycle
	Opts               []grpc.DialOption             `group:"grpc_client_options"`
	Reloader           *fxcert_reloader.CertReloader `optional:"true" name:"grpc_conn_manager"`
	UnaryInterceptors  []*UnaryClientInterceptor     `group:"unary_client_interceptor"`
	StreamInterceptors []*StreamClientInterceptor    `group:"stream_client_interceptor"`
}

func ProvideConnManager(p ConnManagerParams) *ConnManager {
	if p.Reloader != nil {
		p.Lc.Append(fx.Hook{OnStart: p.Reloader.Start, OnStop: p.Reloader.Stop})
	}
	output := NewConnManager(append(
		p.Opts,
		WithUnaryClientInterceptors(p.UnaryInterceptors),
		WithStreamClientInterceptors(p.StreamInterceptors),
	))
	p.Lc.Append(fx.Hook{OnStop: output.Stop})
	return output
}

func (m *ConnManager) Get(address string) (*grpc.ClientConn, error) {
	m.lock.RLock()
	conn, ok := m.idx[address]
	m.lock.RUnlock()
	if !ok {
		return m.createConnection(address)
	}
	return conn, nil
}

func (m *ConnManager) createConnection(address string) (*grpc.ClientConn, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	// Check again, to avoid a race condition where we try to create the same connection concurrently
	conn, ok := m.idx[address]
	if ok {
		return conn, nil
	}
	conn, err := grpc.NewClient(address, m.opts...)
	if err != nil {
		return nil, fmt.Errorf("clientManager: createConnection: %w", err)
	}
	m.idx[address] = conn
	return conn, nil
}
