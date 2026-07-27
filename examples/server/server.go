package server

import (
	"net/http"

	pb "google.golang.org/grpc/examples/route_guide/routeguide"
)

type RouteGuideServer struct {
	pb.UnimplementedRouteGuideServer
}

// NewServer is the constructor used by the fx system
// We use the grpc interface as return type to prevent having to typecast
// (fx cannot automatically cast a struct to the interfaces it implements)
func NewServer() pb.RouteGuideServer {
	return &RouteGuideServer{}
}

// NewHttpMux provides a handler for an http.Server
func NewHttpMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func RpcMapper(req *http.Request) string {
	return "get-health"
}
