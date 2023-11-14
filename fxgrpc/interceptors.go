package fxgrpc

import (
	"sort"

	"google.golang.org/grpc"
)

// UnaryClientInterceptor wraps a grpc.UnaryClientInterceptor with a weight that determines its position in the interceptor chain
type UnaryClientInterceptor struct {
	Weight      uint
	Interceptor grpc.UnaryClientInterceptor
}

// UnaryServerInterceptor wraps a grpc.UnaryServerInterceptor with a weight that determines its position in the interceptor chain
type UnaryServerInterceptor struct {
	Weight      uint
	Interceptor grpc.UnaryServerInterceptor
}

// StreamClientInterceptor wraps a grpc.StreamClientInterceptor with a weight that determines its position in the interceptor chain
type StreamClientInterceptor struct {
	Weight      uint
	Interceptor grpc.StreamClientInterceptor
}

// StreamServerInterceptor wraps a grpc.StreamServerInterceptor with a weight that determines its position in the interceptor chain
type StreamServerInterceptor struct {
	Weight      uint
	Interceptor grpc.StreamServerInterceptor
}

func WithUnaryClientInterceptors(ui []*UnaryClientInterceptor) grpc.DialOption {
	unaryIx := make([]grpc.UnaryClientInterceptor, 0, len(ui))
	for _, ix := range SortInterceptors(ui) {
		unaryIx = append(unaryIx, ix.Interceptor)
	}
	return grpc.WithChainUnaryInterceptor(unaryIx...)
}

func WithStreamClientInterceptors(ui []*StreamClientInterceptor) grpc.DialOption {
	streamIx := make([]grpc.StreamClientInterceptor, 0, len(ui))
	for _, ix := range SortInterceptors(ui) {
		streamIx = append(streamIx, ix.Interceptor)
	}
	return grpc.WithChainStreamInterceptor(streamIx...)
}

type WeightedInterceptor interface {
	GetWeight() uint
}

func (i *UnaryClientInterceptor) GetWeight() uint {
	return i.Weight
}

func (i *UnaryServerInterceptor) GetWeight() uint {
	return i.Weight
}

func (i *StreamClientInterceptor) GetWeight() uint {
	return i.Weight
}

func (i *StreamServerInterceptor) GetWeight() uint {
	return i.Weight
}

type WeightedInterceptors []WeightedInterceptor

func (w WeightedInterceptors) Len() int           { return len(w) }
func (w WeightedInterceptors) Swap(i, j int)      { w[i], w[j] = w[j], w[i] }
func (w WeightedInterceptors) Less(i, j int) bool { return w[i].GetWeight() < w[j].GetWeight() }

// SortInterceptors will order the slice of given interceptors and returns the ordered slice
// Items will be sorted in ascending weight
// Any nil items will be removed
// It is some sugar around sort.Sort to help with the type system checks
// The interceptor list should never be so large that the performance of this function matters
func SortInterceptors[T WeightedInterceptor](list []T) []T {
	iList := make([]WeightedInterceptor, len(list))
	// Copy to into a new slice to make the type checker happy
	for i := range list {
		iList[i] = list[i]
	}
	// Remove any nil elements, disregarding order
	for i := 0; i < len(iList); i++ {
		if iList[i] == nil {
			iList[i] = iList[0]
			iList = iList[1:]
		}
	}
	sort.Sort(WeightedInterceptors(iList))
	// Copy in original, because type checker
	for i := range iList {
		list[i] = iList[i].(T)
	}
	return list[:len(iList)]
}
