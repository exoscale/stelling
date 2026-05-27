package interceptor

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/exoscale/stelling/fxhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func splitMethod(fullMethod string) (string, string) {
	fullMethod = strings.TrimPrefix(fullMethod, "/") // remove leading slash
	if service, method, ok := strings.Cut(fullMethod, "/"); ok {
		return service, method
	}
	return "unknown", "unknown"
}

type GrpcAuthorizer interface {
	Check(ctx context.Context, service string, method string) (bool, error)
}

type HttpAuthorizer interface {
	Check(req *http.Request, method string) (bool, error)
}

// NewAuthorizerUnaryServerInterceptor returns a UnaryServerInterceptor which evaluates the Authorizer policy for each request
// If the policy check fails a PermissionDenied error code is returned, otherwise the request handler is executes as normal
func NewAuthorizerUnaryServerInterceptor(a GrpcAuthorizer) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		service, method := splitMethod(info.FullMethod)
		if ok, err := a.Check(ctx, service, method); !ok {
			return nil, status.Errorf(codes.PermissionDenied, "policy denied")
		} else if err != nil {
			return nil, status.Errorf(codes.Internal, "authorization failed: %s", err)
		}
		return handler(ctx, req)
	}
}

// NewAuthorizerStreamServerInterceptor returns a StreamServerInterceptor which evaluates the Authorizer policy for each request
// If the policy check fails a PermissionDenied error code is returned, otherwise the request handler is executes as normal
func NewAuthorizerStreamServerInterceptor(a GrpcAuthorizer) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		service, method := splitMethod(info.FullMethod)
		if ok, err := a.Check(ctx, service, method); !ok {
			return status.Errorf(codes.PermissionDenied, "policy denied")
		} else if err != nil {
			return status.Errorf(codes.Internal, "authorization failed: %s", err)
		}
		return handler(srv, ss)
	}
}

// NewAuthorizerHandler returns an HTTP handler which evaluates the Authorizer policy as an http middleware
// If the policy check fails StatusForbidden is returned, otherwise the wrapped handler is executed
func NewAuthorizerHandler(a HttpAuthorizer, wrapped http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := fxhttp.RPCMethodFromContext(r.Context())
		if method == "" {
			method = "unknown"
		}
		if ok, err := a.Check(r, method); !ok {
			w.WriteHeader(http.StatusForbidden)
			return
		} else if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "authorization failed: %s", err)
			return
		}
		wrapped.ServeHTTP(w, r)
	})
}
