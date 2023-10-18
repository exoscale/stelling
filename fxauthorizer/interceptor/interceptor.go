package interceptor

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func splitMethod(fullMethod string) (string, string) {
	fullMethod = strings.TrimPrefix(fullMethod, "/") // remove leading slash
	if i := strings.Index(fullMethod, "/"); i >= 0 {
		return fullMethod[:i], fullMethod[i+1:]
	}
	return "unknown", "unknown"
}

type Authorizer interface {
	Check(ctx context.Context, service string, method string) (bool, error)
}

// NewAuthorizerUnaryServerInterceptor returns a UnaryServerInterceptor which evaluates the Authorizer policy for each request
// If the policy check fails a PermissionDenied error code is returned, otherwise the request handler is executes as normal
func NewAuthorizerUnaryServerInterceptor(a Authorizer) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		service, method := splitMethod(info.FullMethod)
		if ok, err := a.Check(ctx, service, method); !ok {
			return nil, status.Errorf(codes.PermissionDenied, "authorization failed: %v", err.Error())
		}
		return handler(ctx, req)
	}
}

// NewAuthorizerStreamServerInterceptor returns a StreamServerInterceptor which evaluates the Authorizer policy for each request
// If the policy check fails a PermissionDenied error code is returned, otherwise the request handler is executes as normal
func NewAuthorizerStreamServerInterceptor(a Authorizer) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		service, method := splitMethod(info.FullMethod)
		if ok, err := a.Check(ctx, service, method); !ok {
			return status.Errorf(codes.PermissionDenied, "authorization failed: %v", err.Error())
		}
		return handler(srv, ss)
	}
}
