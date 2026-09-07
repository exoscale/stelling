package fxlogging

import (
	"net/http"

	"github.com/exoscale/stelling/fxhttp"
	"github.com/exoscale/stelling/fxlogging/interceptor"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type HttpMiddlewareResult struct {
	fx.Out

	Middleware *fxhttp.Middleware `group:"http_middleware"`
}

func NewHttpRequestLoggerMiddleware(logger *zap.Logger, opts ...interceptor.HTTPOption) HttpMiddlewareResult {
	return HttpMiddlewareResult{
		Middleware: &fxhttp.Middleware{
			Handler: func(next http.Handler) http.Handler {
				return interceptor.NewRequestLogger(logger, next, opts...)
			},
			Weight: 40,
		},
	}
}

func NewHttpPanicLoggerMiddleware(logger *zap.Logger, opts ...interceptor.HTTPOption) HttpMiddlewareResult {
	return HttpMiddlewareResult{
		Middleware: &fxhttp.Middleware{
			Handler: func(next http.Handler) http.Handler {
				return interceptor.NewPanicLogger(logger, next)
			},
			Weight: 39,
		},
	}
}
