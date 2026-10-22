package openapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/exoscale/stelling/fxhttp"
	"github.com/exoscale/stelling/fxhttp/apierror"
)

type options struct {
	requestErrorHandler func(http.ResponseWriter, *http.Request, error)
	errorMapper         func(context.Context, error) *fxhttp.Error
	status              func(apierror.Category) int
	errorTypes          []*apierror.Type
	errorExtensions     []errorExtension
	contractViolation   func(r *http.Request, operationID string, e *fxhttp.Error)
}

type Option func(*options)

// WithRequestErrorHandler replaces the handler writing the response of requests that fail validation,
// decoding or binding. By default they are written as a 400 by fxhttp.WriteError
func WithRequestErrorHandler(h func(http.ResponseWriter, *http.Request, error)) Option {
	return func(o *options) {
		o.requestErrorHandler = h
	}
}

// WithErrorMapper converts the errors returned by handlers into the error response written for them, eg for errors
// which are not API errors (see the apierror package), or to observe them
// When it returns nil, the error is written as is: API errors with the status of their category, see
// WithStatusMapping, and any other error as a 500
func WithErrorMapper(m func(context.Context, error) *fxhttp.Error) Option {
	return func(o *options) {
		o.errorMapper = m
	}
}

// WithStatusMapping sets the http status of the API errors of each category, both in the error responses and in
// the rendered specification. It defaults to fxhttp.DefaultStatus
func WithStatusMapping(status func(apierror.Category) int) Option {
	return func(o *options) {
		o.status = status
	}
}

// WithErrorTypes registers the error types operations can list in x-errors, see ErrorsExtension
// The rendered specification documents them in the error responses of each operation
func WithErrorTypes(types ...*apierror.Type) Option {
	return func(o *options) {
		o.errorTypes = append(o.errorTypes, types...)
	}
}

// WithErrorExtension documents a service specific member of the error responses in the rendered specification
// schemaComponent is the name of the component schema of the specification describing the member
func WithErrorExtension(name, schemaComponent string) Option {
	return func(o *options) {
		o.errorExtensions = append(o.errorExtensions, errorExtension{name: name, schema: schemaComponent})
	}
}

// WithContractViolationHandler is called when an operation listing its error types in x-errors writes an
// error response with an error type it does not declare, or with another status than the one of the error type
// Request validation failures and server errors (5xx) are always allowed
// Use it to fail tests, or to log violations in production
func WithContractViolationHandler(h func(r *http.Request, operationID string, e *fxhttp.Error)) Option {
	return func(o *options) {
		o.contractViolation = h
	}
}

func (o *options) catalog() (*errorCatalog, error) {
	return newErrorCatalog(o.errorTypes, o.status)
}

// writeResponseError writes the error returned by a handler, through the error mapper
func (o *options) writeResponseError(w http.ResponseWriter, r *http.Request, err error) {
	if o.errorMapper != nil {
		if e := o.errorMapper(r.Context(), err); e != nil {
			err = e
		}
	}
	if _, ok := errors.AsType[*fxhttp.Error](err); !ok {
		if p := apierror.From(err); p != nil {
			err = fxhttp.ErrorFromAPIError(p, o.status(apierror.CategoryOf(p)))
		}
	}
	fxhttp.WriteError(w, r, err)
}

func newOptions(opts []Option) *options {
	o := &options{
		requestErrorHandler: writeBadRequest,
		status:              fxhttp.DefaultStatus,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// writeBadRequest writes err as a 400 error response
func writeBadRequest(w http.ResponseWriter, r *http.Request, err error) {
	fxhttp.WriteError(w, r, &fxhttp.Error{Status: http.StatusBadRequest, Detail: err.Error(), Cause: err})
}
