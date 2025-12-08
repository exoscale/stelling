package interceptor

import (
	"context"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

// loggerWithMetadata extracts values from gRPC incoming metadata and adds them to the
// provided logger as string fields. mdFields maps metadata key -> logger field key.
// If mdFields is nil or empty, the logger is returned unchanged.
func loggerWithMetadata(ctx context.Context, logger *zap.Logger, mdFields map[string]string) *zap.Logger {
	if len(mdFields) == 0 {
		return logger
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return logger
	}
	for mdKey, fieldKey := range mdFields {
		if vals := md.Get(mdKey); len(vals) > 0 {
			logger = logger.With(zap.String(fieldKey, vals[0]))
		}
	}
	return logger
}

// loggerWithDefaultFields adds standard gRPC fields to the provided logger
func loggerWithDefaultFields(ctx context.Context, logger *zap.Logger, info *otelgrpc.InterceptorInfo) *zap.Logger {
	traceid, _ := traceIdFromContext(ctx)

	// TODO: refactor this using otel.semconv
	service, method := MethodFromInterceptorInfo(info)

	return logger.With(
		zap.String("otlp.trace_id", traceid),
		zap.String("rpc.system", "grpc"),
		zap.String("service.name", serviceName()),
		zap.String("rpc.method", method),
		zap.String("rpc.service", service))
}
