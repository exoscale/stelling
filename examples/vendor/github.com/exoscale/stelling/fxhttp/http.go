// package fxhttp provides a convenient way to create well behaved http servers
package fxhttp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	reloader "github.com/exoscale/stelling/fxcert-reloader"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type serverModuleOpts struct {
	name   string
	mapper func(*http.Request) string
}

type serverModuleOption func(*serverModuleOpts)

// WithServerModuleName will annotate the outputs with the given name
func WithServerModuleName(name string) serverModuleOption {
	return func(o *serverModuleOpts) {
		o.name = name
	}
}

// WithRPCMethodMapper adds a function that maps request to the name of the rpc.method
// This information is used by various observability modules to annotate the data
func WithRPCMethodMapper(mapper func(*http.Request) string) serverModuleOption {
	return func(o *serverModuleOpts) {
		o.mapper = mapper
	}
}

// server is a tuple of http.Server with its accompanying address or socket name
type server struct {
	server     *http.Server
	addr       string
	socketName string
}

func newServer(s *http.Server, conf ServerConfig) *server {
	return &server{s, conf.HttpServerConfig().Address, conf.HttpServerConfig().SocketName}
}

// NewModule provides a configured *http.Server to the system
// You still have to invoke StartHttpServer to ensure it starts
func NewModule(conf ServerConfig, sOpts ...serverModuleOption) fx.Option {
	modOpts := &serverModuleOpts{}
	for _, o := range sOpts {
		o(modOpts)
	}

	opts := fx.Options()
	if modOpts.mapper != nil {
		opts = fx.Options(
			fx.Supply(RPCMethodMapper(modOpts.mapper)),
		)
	}
	if modOpts.name == "" {
		opts = fx.Options(
			opts,
			fx.Supply(
				fx.Annotate(conf, fx.As(new(ServerConfig))),
				fx.Private,
			),
			fx.Provide(
				fx.Annotate(NewHTTPServer, fx.ParamTags(``, ``, `optional:"true"`, ``, `group:"http_middleware"`)),
				newServer,
				fx.Annotate(
					NewInjectRPCMethodMiddleware,
					fx.ParamTags(`optional:"true"`),
					fx.ResultTags(`group:"http_middleware"`),
				),
			),
		)
	} else {
		nameTag := fmt.Sprintf("name:\"%s\"", modOpts.name)
		opts = fx.Options(
			opts,
			fx.Supply(
				fx.Annotate(conf, fx.As(new(ServerConfig)), fx.ResultTags(nameTag)),
				fx.Private,
			),
			fx.Provide(
				fx.Annotate(
					NewHTTPServer,
					// We only want to add the middleware to the "main" http server
					// So we request a different group when a named server is created: this will ensure we always
					// receive an empty middleware list
					fx.ParamTags(``, nameTag, `optional:"true"`, nameTag, `group:"empty_http_middleware"`),
					fx.ResultTags(nameTag),
				),
				fx.Annotate(
					newServer,
					fx.ParamTags(nameTag, nameTag),
					fx.ResultTags(nameTag),
				),
			),
		)
	}
	if conf.HttpServerConfig().TLS {
		opts = fx.Options(
			opts,
			fx.Provide(
				GetCertReloaderConfig,
				reloader.ProvideCertReloader,
				fx.Private,
			),
		)
	}
	moduleName := "http"
	if modOpts.name != "" {
		moduleName = fmt.Sprintf("%s-%s", modOpts.name, moduleName)
	}
	return fx.Module(moduleName, opts)
}

// NewNamedModule adds a named http server to the system
// It is mostly of use for other stelling components which may need to
// start their own http servers
// The average application should be able to use NewModule instead
//
// Deprecated: please use [NewModule] with the [WithServerModuleName] option
// and add `fx.Invoke(fx.Annotate(StartHttpServer), fx.ParamTags("", "name=\"yourname\"", ""))` to the system
func NewNamedModule(name string, conf ServerConfig) fx.Option {
	nameTag := fmt.Sprintf("name:\"%s\"", name)
	return fx.Options(
		NewModule(conf, WithServerModuleName(name)),
		// We're not putting this in the module, so that the module which
		// embeds this can chose when the http server should start
		fx.Invoke(fx.Annotate(StartHttpServer, fx.ParamTags(``, nameTag, ``))),
	)
}

type ServerConfig interface {
	HttpServerConfig() *Server
}

type Server struct {
	// A systemd socket name. Takes precedence over Address
	// In order to simplify, only systemd-activated socket with names are allowed, even if it is
	// just one socket
	SocketName string
	// Address is the address+port the server will bind to, as passed to net.Listen
	// If the Address starts with a / we will create unix domainsocket listener
	Address string `default:"localhost:8080" validate:"tcp_addr|unix_addr"`
	// TLS indicates whether the http server exposes with TLS
	TLS bool
	// CertFile is the path to the pem encoded TLS certificate
	CertFile string `validate:"required_if=TLS true,omitempty,file"`
	// KeyFile is the path to the pem encoded private key of the TLS certificate
	KeyFile string `validate:"required_if=TLS true,omitempty,file"`
	// ClientCAFile is the path to a pem encoded CA cert bundle used to validate clients
	ClientCAFile string `validate:"excluded_without=TLS,omitempty,file"`
}

func (s *Server) HttpServerConfig() *Server {
	return s
}

func (s *Server) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if s == nil {
		return nil
	}

	enc.AddString("socket-name", s.SocketName)
	enc.AddString("address", s.Address)
	enc.AddBool("tls", s.TLS)

	if s.TLS {
		enc.AddString("cert-file", s.CertFile)
		enc.AddString("key-file", s.KeyFile)
		enc.AddString("client-ca-file", s.ClientCAFile)
	}

	return nil
}

func GetCertReloaderConfig(conf ServerConfig) *reloader.CertReloaderConfig {
	return &reloader.CertReloaderConfig{
		CertFile:       conf.HttpServerConfig().CertFile,
		KeyFile:        conf.HttpServerConfig().KeyFile,
		ReloadInterval: 1 * time.Hour,
	}
}

func NewListener(ctx context.Context, socketName string, addr string) (net.Listener, error) {
	if socketName != "" {
		return NamedSocketListener(socketName)
	} else {
		var lc net.ListenConfig
		if strings.HasPrefix(addr, "/") || strings.HasPrefix(addr, ".") {
			// Remove the previous socket in case one was left behind by an abrupt process termination
			if err := os.Remove(addr); err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			return lc.Listen(ctx, "unix", addr)
		}
		return lc.Listen(ctx, "tcp", addr)
	}
}

func NewHTTPServer(lc fx.Lifecycle, conf ServerConfig, r *reloader.CertReloader, handler http.Handler, middleware []*Middleware) (*http.Server, error) {
	server := &http.Server{}

	if conf.HttpServerConfig().TLS {
		tlsConf, err := reloader.MakeServerTLS(r, conf.HttpServerConfig().ClientCAFile)
		if err != nil {
			return nil, err
		}
		server.TLSConfig = tlsConf
	}

	middleware = slices.DeleteFunc(middleware, func(a *Middleware) bool { return a == nil })
	slices.SortFunc(middleware, func(a, b *Middleware) int { return b.Weight - a.Weight })
	for _, mw := range middleware {
		handler = mw.Handler(handler)
	}
	server.Handler = handler

	return server, nil
}

func StartHttpServer(lc fx.Lifecycle, s *server, logger *zap.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting http server", zap.String("address", s.addr))
			lis, err := NewListener(ctx, s.socketName, s.addr)
			if err != nil {
				return err
			}
			if s.server.TLSConfig != nil {
				go func() {
					if err := s.server.ServeTLS(lis, "", ""); err != http.ErrServerClosed {
						logger.Fatal("Error while serving http", zap.Error(err))
					} else {
						logger.Info("Done serving http")
					}
				}()
			} else {
				go func() {
					if err := s.server.Serve(lis); err != http.ErrServerClosed {
						logger.Fatal("Error while serving http", zap.Error(err))
					} else {
						logger.Info("Done serving http")
					}
				}()
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping http server")
			return s.server.Shutdown(ctx)
		},
	})
}

type Middleware struct {
	Handler func(http.Handler) http.Handler
	Weight  int
}

// RPCMethodMapper returns a fully qualified rpc method name for a given http request
type RPCMethodMapper func(req *http.Request) string

type rpcMethodContextKey struct{}

var rpcMethodCtxKey *rpcMethodContextKey = &rpcMethodContextKey{}

func NewInjectRPCMethodMiddleware(mapper RPCMethodMapper) *Middleware {
	if mapper == nil {
		return nil
	}
	return &Middleware{
		Handler: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if rpcMethod := mapper(r); rpcMethod != "" {
					r = r.WithContext(injectRPCMethod(r.Context(), rpcMethod))
				}
				next.ServeHTTP(w, r)
			})
		},
		Weight: 10,
	}
}

func injectRPCMethod(ctx context.Context, rpcMethod string) context.Context {
	return context.WithValue(ctx, rpcMethodCtxKey, rpcMethod)
}

// RPCMethodFromContext returns the current rpc.method stored in the context
// Returns the empty string if not present
func RPCMethodFromContext(ctx context.Context) string {
	rpcMethod, ok := ctx.Value(rpcMethodCtxKey).(string)
	if !ok {
		return ""
	}
	return rpcMethod
}
