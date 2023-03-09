package server

import pb "google.golang.org/grpc/examples/route_guide/routeguide"

type RouteGuideServer struct {
	pb.UnimplementedRouteGuideServer
}

// NewServer is the constructor used by the fx system
// We use the grpc interface as return type to prevent having to typecast
// (fx cannot automatically cast a struct to the interfaces it implements)
func NewServer() pb.RouteGuideServer {
	return &RouteGuideServer{}
}
