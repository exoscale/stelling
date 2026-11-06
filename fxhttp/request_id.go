package fxhttp

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// RequestIDHeader is the header carrying the id used to correlate a request across services
const RequestIDHeader = "x-request-id"

type requestIDContextKey struct{}

var requestIDCtxKey = &requestIDContextKey{}

// maxRequestIDLength bounds the incoming request ids: they are logged with every request
const maxRequestIDLength = 128

// newRequestIDMiddleware ensures every request carries a request id
// The id of the incoming RequestIDHeader is used, or a new one is generated when it is absent or invalid
// The id is set on the request header, so it is picked up by components reading the header, and stored in the context
func newRequestIDMiddleware() *Middleware {
	return &Middleware{
		Handler: requestIDHandler,
		// The request id must be known to the logging and tracing middleware
		Weight: 15,
	}
}

func requestIDHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(RequestIDHeader)
		if !validRequestID(requestID) {
			requestID = uuid.NewString()
			r.Header.Set(RequestIDHeader, requestID)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDCtxKey, requestID)))
	})
}

// validRequestID reports whether id is a non empty, bounded, printable ascii string
func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLength {
		return false
	}
	for i := range len(id) {
		if id[i] < 0x21 || id[i] > 0x7e {
			return false
		}
	}
	return true
}

// RequestIDFromContext returns the request id stored by the request id middleware
// Returns the empty string if not present
func RequestIDFromContext(ctx context.Context) string {
	requestID, ok := ctx.Value(requestIDCtxKey).(string)
	if !ok {
		return ""
	}
	return requestID
}
