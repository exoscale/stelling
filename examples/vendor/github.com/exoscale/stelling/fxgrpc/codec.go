package fxgrpc

import (

	// use the v2 proto package we can continue serializing
	// messages from our dependencies that don't use vtproto
	"google.golang.org/grpc/encoding"
	// Guarantee that the built-in proto is called registered before this one
	// so that it can be replaced.
	_ "google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/mem"
)

// Name is the name registered for the proto compressor.
const Name = "proto"

type vtprotoMessage interface {
	MarshalToSizedBufferVT(data []byte) (int, error)
	UnmarshalVT([]byte) error
	SizeVT() int
}

type codec struct {
	fallback encoding.CodecV2
}

func (codec) Name() string { return Name }

// For the moment there's no way for consumer of stelling to change the memory pool used to hold serialized messages
// But I'm not sure there's really a need for that
var defaultBufferPool = mem.DefaultBufferPool()

func (c *codec) Marshal(v any) (mem.BufferSlice, error) {
	if m, ok := v.(vtprotoMessage); ok {
		size := m.SizeVT()
		if mem.IsBelowBufferPoolingThreshold(size) {
			buf := make([]byte, size)
			if _, err := m.MarshalToSizedBufferVT(buf[:size]); err != nil {
				return nil, err
			}
			return mem.BufferSlice{mem.SliceBuffer(buf)}, nil
		}
		buf := defaultBufferPool.Get(size)
		if _, err := m.MarshalToSizedBufferVT((*buf)[:size]); err != nil {
			defaultBufferPool.Put(buf)
			return nil, err
		}
		return mem.BufferSlice{mem.NewBuffer(buf, defaultBufferPool)}, nil
	}

	return c.fallback.Marshal(v)
}

func (c *codec) Unmarshal(data mem.BufferSlice, v any) error {
	if m, ok := v.(vtprotoMessage); ok {
		buf := data.MaterializeToBuffer(defaultBufferPool)
		defer buf.Free()
		return m.UnmarshalVT(buf.ReadOnlyData())
	}

	return c.fallback.Unmarshal(data, v)
}

func init() {
	encoding.RegisterCodecV2(&codec{
		fallback: encoding.GetCodecV2("proto"),
	})
}
