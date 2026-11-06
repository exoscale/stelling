package fxhttp

import (
	"context"
	"net/http"
)

// IAMIdentityIDHeader is the header in which the Exoscale public API forwards the id of the
// authenticated IAM identity to the services behind it
const IAMIdentityIDHeader = "x-exoscale-iam-identity-id"

type iamIdentityIDContextKey struct{}

var iamIdentityIDCtxKey = &iamIdentityIDContextKey{}

// IAMIdentityHandler stores the IAMIdentityIDHeader of the request in its context
// Only install it on routes that are exclusively reachable through the public API, which sets the header:
// the header is trusted as is
func IAMIdentityHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if identityID := r.Header.Get(IAMIdentityIDHeader); identityID != "" {
			r = r.WithContext(context.WithValue(r.Context(), iamIdentityIDCtxKey, identityID))
		}
		next.ServeHTTP(w, r)
	})
}

// IAMIdentityIDFromContext returns the IAM identity id stored by IAMIdentityHandler
// Returns the empty string if not present
func IAMIdentityIDFromContext(ctx context.Context) string {
	identityID, ok := ctx.Value(iamIdentityIDCtxKey).(string)
	if !ok {
		return ""
	}
	return identityID
}
