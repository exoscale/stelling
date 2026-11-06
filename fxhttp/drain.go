package fxhttp

import (
	"net/http"
	"sync/atomic"
)

// DrainState reports whether the process is shutting down
// The http server enters draining as the first step of its shutdown, before it stops accepting
// connections. Other components can use it to stop admitting new work, or to fail readiness checks
type DrainState struct {
	draining atomic.Bool
}

func NewDrainState() *DrainState {
	return &DrainState{}
}

// EnterDraining marks the process as draining and reports whether this call changed the state
func (s *DrainState) EnterDraining() bool {
	return s.draining.CompareAndSwap(false, true)
}

// IsDraining reports whether the process has entered draining mode
func (s *DrainState) IsDraining() bool {
	return s != nil && s.draining.Load()
}

// newDrainMiddleware discourages HTTP/1.x connection reuse once draining begins,
// so clients reconnect to another instance
func newDrainMiddleware(drainState *DrainState) *Middleware {
	return &Middleware{
		Handler: DrainHandler(drainState),
		Weight:  5,
	}
}

// DrainHandler returns the handler of the drain middleware the module installs, for use outside of an fx system
func DrainHandler(drainState *DrainState) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if drainState.IsDraining() && r.ProtoMajor == 1 {
				w.Header().Set("Connection", "close")
			}
			next.ServeHTTP(w, r)
		})
	}
}
