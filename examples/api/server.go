package api

import (
	"net/http"

	"github.com/exoscale/stelling/examples/api/internal"
	"github.com/exoscale/stelling/examples/api/public"
	"github.com/exoscale/stelling/examples/config"
	"go.uber.org/zap"
)

// APIServer is the single server instance that serves both the public and internal APIs.
// Individual handler methods should be defined in separate files.
// See the healthz and greeting examples.
type APIServer struct {
	logger  *zap.Logger
	message string
}

func NewAPIServer(
	conf *config.Config,
	logger *zap.Logger,
) *APIServer {
	return &APIServer{
		logger:  logger,
		message: conf.Greeting.Message,
	}
}

func NewAPIHTTPMux(s *APIServer) http.Handler {
	mux := http.NewServeMux()

	// Register public handlers
	public.HandlerFromMux(public.NewStrictHandler(s, nil), mux)

	// Register internal handlers
	internal.HandlerFromMuxWithBaseURL(internal.NewStrictHandler(s, nil), mux, "/internal")

	return mux
}
