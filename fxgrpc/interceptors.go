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

// SortInterceptors will order the slice of given interceptors by mutating it in place
// Items will be sorted in ascending weight
// It is some sugar around sort.Sort to help with the type system checks
// The interceptor list should never be so large that the performance of this function matters
func SortInterceptors[T WeightedInterceptor](list []T) {
	iList := make([]WeightedInterceptor, len(list))
	for i := range list {
		iList[i] = list[i]
	}
	sort.Sort(WeightedInterceptors(iList))
}
