package interceptor

import (
	"context"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	svcNameKey = "OTEL_SERVICE_NAME"
)

// serviceName tries to obtain a valid service name
// It will try, in this order: the environment, the process name or default to "unknown_service"
// It will honor opentelemetry resource conventions and environment variables
func serviceName() string {
	svcName := strings.TrimSpace(os.Getenv(svcNameKey))
	if svcName != "" {
		return svcName
	}

	if exec, err := os.Executable(); err != nil && exec != "" {
		return path.Base(exec)
	}

	return "unknown_service"
}

// peerService will extract the peer.service metadata from the context, if any
// If more than 1 value is set on the request metadata, the last value will be returned
func peerService(ctx context.Context) (string, bool) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if ps, ok := md[peerServiceMDKey]; ok {
			return ps[len(ps)-1], true
		}
	}
	return "", false
}

func splitMethod(fullMethod string) (string, string) {
	fullMethod = strings.TrimPrefix(fullMethod, "/") // remove leading slash
	if i := strings.Index(fullMethod, "/"); i >= 0 {
		return fullMethod[:i], fullMethod[i+1:]
	}
	return "unknown", "unknown"
}

func MethodFromInterceptorInfo(info *otelgrpc.InterceptorInfo) (string, string) {
	switch info.Type {
	case otelgrpc.StreamClient, otelgrpc.UnaryClient:
		return splitMethod(info.Method)
	case otelgrpc.StreamServer:
		return splitMethod(info.StreamServerInfo.FullMethod)
	case otelgrpc.UnaryServer:
		return splitMethod(info.UnaryServerInfo.FullMethod)
	}
	return "unknown", "unknown"
}

type reporter struct {
	svcName   string
	conf      *interceptorConfig
	info      *otelgrpc.InterceptorInfo
	startTime time.Time
	logger    *zap.Logger
}

func (r *reporter) Log(ctx context.Context, payload any, handleErr error) {
	duration := time.Since(r.startTime)
	code := status.Code(handleErr)
	level := r.conf.levelFunc(r.info, code)
	traceid, _ := traceIdFromContext(ctx)

	// TODO: refactor this using otel.semconv
	service, method := MethodFromInterceptorInfo(r.info)
	logger := r.logger.With(
		zap.String("rpc.system", "grpc"),
		zap.String("service.name", r.svcName),
		zap.String("rpc.method", method),
		zap.String("rpc.service", service),
		zap.Time("rpc.request.start_time", r.startTime),
		zap.String("rpc.grpc.status_code", code.String()),
		zap.Duration("rpc.request.duration", duration),
		zap.String("otlp.trace_id", traceid),
	)
	if deadline, ok := ctx.Deadline(); ok {
		logger = logger.With(zap.Time("rpc.request.deadline", deadline))
	}
	// TODO: Only on server maybe?
	if peerInfo, ok := peer.FromContext(ctx); ok {
		if tcpAddr, ok := peerInfo.Addr.(*net.TCPAddr); ok {
			logger = logger.With(
				zap.String("sock.net.peer.address", tcpAddr.IP.String()),
				zap.Int("sock.net.peer.port", tcpAddr.Port),
			)
		} else {
			logger = logger.With(zap.String("sock.net.peer.address", peerInfo.Addr.String()))
		}
	}
	if peerService, ok := peerService(ctx); ok {
		logger = logger.With(zap.String("peer.service", peerService))
	}
	logger = r.conf.extraFieldsFunc(logger, r.info, payload)
	if payload != nil && r.conf.payloadFilter(r.info) {
		p, ok := payload.(proto.Message)
		if !ok {
			logger.DPanic("payload is not a google.golang.org/protobuf/proto.Message", zap.Any("msg", payload))
		} else {
			logger = logger.With(zap.Any("rpc.request.content", p))
		}
	}
	if handleErr != nil {
		logger = logger.With(zap.Error(handleErr))
	}

	logger.Log(level, "finished call")
}

func NewLoggingUnaryServerInterceptor(logger *zap.Logger, opts ...Option) grpc.UnaryServerInterceptor {
	svcName := serviceName()
	conf := newInterceptorConfig(opts)
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		startTime := time.Now()

		resp, err := handler(ctx, req)

		interceptorInfo := &otelgrpc.InterceptorInfo{UnaryServerInfo: info, Type: otelgrpc.UnaryServer}
		if !conf.logFilter(interceptorInfo) {
			return resp, err
		}

		r := &reporter{
			svcName:   svcName,
			conf:      conf,
			info:      interceptorInfo,
			startTime: startTime,
			logger:    logger,
		}

		r.Log(ctx, req, err)

		return resp, err
	}
}

type monitoredServerStream struct {
	grpc.ServerStream
	ctx     context.Context
	payload any
}

func (s *monitoredServerStream) Context() context.Context {
	return s.ctx
}

func (s *monitoredServerStream) RecvMsg(m any) error {
	err := s.ServerStream.RecvMsg(m)
	// We only store the first payload: this is the request for a ServerStream
	// A dedicated interceptor should be used to log all events on a stream
	if err == nil && s.payload == nil {
		msg, ok := m.(proto.Message)
		if ok {
			// With vtproto the msg pointers can be reused, so let's clone
			// to ensure we don't write garbage in the log
			s.payload = proto.Clone(msg)
		}
	}
	return err
}

func NewLoggingStreamServerInterceptor(logger *zap.Logger, opts ...Option) grpc.StreamServerInterceptor {
	svcName := serviceName()
	conf := newInterceptorConfig(opts)
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		startTime := time.Now()
		ctx := ss.Context()

		mStream := &monitoredServerStream{ctx: ctx, ServerStream: ss}

		err := handler(srv, mStream)

		interceptorInfo := &otelgrpc.InterceptorInfo{StreamServerInfo: info, Type: otelgrpc.StreamServer}
		if !conf.logFilter(interceptorInfo) {
			return err
		}

		r := &reporter{
			svcName:   svcName,
			conf:      conf,
			info:      interceptorInfo,
			startTime: startTime,
			logger:    logger,
		}
		r.Log(ctx, mStream.payload, err)

		return err
	}
}

func NewLoggingUnaryClientInterceptor(logger *zap.Logger, opts ...Option) grpc.UnaryClientInterceptor {
	svcName := serviceName()
	conf := newInterceptorConfig(append([]Option{WithLevelFunc(DefaultClientCodeToLevel)}, opts...))
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, callopts ...grpc.CallOption) error {
		startTime := time.Now()

		err := invoker(ctx, method, req, reply, cc, callopts...)

		interceptorInfo := &otelgrpc.InterceptorInfo{Method: method, Type: otelgrpc.UnaryClient}
		if !conf.logFilter(interceptorInfo) {
			return err
		}

		r := &reporter{
			svcName:   svcName,
			conf:      conf,
			info:      interceptorInfo,
			startTime: startTime,
			logger:    logger,
		}

		r.Log(ctx, req, err)

		return err
	}
}

type monitoredClientStream struct {
	grpc.ClientStream
	ctx      context.Context
	payload  any
	reporter *reporter
	desc     *grpc.StreamDesc
}

func (s *monitoredClientStream) Context() context.Context {
	return s.ctx
}

// This implementation won't log all the errors, but chances are the client code will anyway
// It will also not log if the stream is finished by canceling the context
// If we do want to change this, here's some inspiration using a channel to communicate the errors for all stream methods
// https://github.com/open-telemetry/opentelemetry-go-contrib/blob/instrumentation/google.golang.org/grpc/otelgrpc/v0.41.1/instrumentation/google.golang.org/grpc/otelgrpc/interceptor.go#L128-L298
// https://github.com/cockroachdb/cockroach/blob/master/pkg/util/tracing/grpcinterceptor/grpc_interceptor.go#L327
// Another good read on how to reliably handle client streams is:
// https://github.com/grpc/grpc-go/issues/5324

func (s *monitoredClientStream) SendMsg(m any) error {
	// Only saving the first message, which we assume is the request
	if s.payload == nil {
		msg, ok := m.(proto.Message)
		if ok {
			// With vtproto the msg pointers can be reused, so let's clone
			// to ensure we don't write garbage in the log
			s.payload = proto.Clone(msg)
		}
	}
	return s.ClientStream.SendMsg(m)
}

func (s *monitoredClientStream) RecvMsg(m any) error {
	err := s.ClientStream.RecvMsg(m)
	if err == nil && !s.desc.ServerStreams {
		s.reporter.Log(s.ctx, s.payload, nil)
		return nil
	}
	if err == io.EOF {
		s.reporter.Log(s.ctx, s.payload, nil)
	} else if err != nil {
		s.reporter.Log(s.ctx, s.payload, err)
	}
	return err
}

func NewLoggingStreamClientInterceptor(logger *zap.Logger, opts ...Option) grpc.StreamClientInterceptor {
	svcName := serviceName()
	conf := newInterceptorConfig(append([]Option{WithLevelFunc(DefaultClientCodeToLevel)}, opts...))
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, callOpts ...grpc.CallOption) (grpc.ClientStream, error) {
		startTime := time.Now()

		cs, err := streamer(ctx, desc, cc, method, callOpts...)

		interceptorInfo := &otelgrpc.InterceptorInfo{Method: method, Type: otelgrpc.StreamClient}
		if !conf.logFilter(interceptorInfo) {
			return cs, err
		}

		r := &reporter{
			svcName:   svcName,
			conf:      conf,
			info:      interceptorInfo,
			startTime: startTime,
			logger:    logger,
		}
		if err != nil {
			r.Log(ctx, nil, err)
			return nil, err
		}

		mStream := &monitoredClientStream{
			ctx:          ctx,
			ClientStream: cs,
			reporter:     r,
			desc:         desc,
		}
		return mStream, nil
	}
}
