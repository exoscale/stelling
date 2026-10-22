package fxhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
)

func TestModuleDrainAndRequestID(t *testing.T) {
	var drain *DrainState
	var srv *http.Server
	var requestID string

	app := fxtest.New(
		t,
		fx.WithLogger(func() fxevent.Logger { return fxevent.NopLogger }),
		NewModule(&Server{Address: "localhost:0"}, WithRequestID()),
		fx.Provide(
			zap.NewNop,
			func() http.Handler {
				return http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
					requestID = RequestIDFromContext(r.Context())
				})
			},
		),
		fx.Invoke(StartHttpServer),
		fx.Populate(&drain, &srv),
	)
	app.RequireStart()

	srv.Handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	require.NotEmpty(t, requestID, "the request id middleware should be installed")
	require.False(t, drain.IsDraining())

	app.RequireStop()
	require.True(t, drain.IsDraining(), "stopping the server should enter draining")
}
