package openapi

import (
	"context"
	"net/http"

	"github.com/exoscale/stelling/fxhttp"
	"github.com/exoscale/stelling/fxhttp/apierror"
)

type options struct {
	requestErrorHandler func(http.ResponseWriter, *http.Request, error)
	errorMapper         func(context.Context, error) *fxhttp.Error
	errorTypes          []*apierror.Type
	errorExtensions     []errorExtension
	contractViolation   func(r *http.Request, operationID string, e *fxhttp.Error)
}

type Option func(*options)

// WithRequestErrorHandler replaces the handler writing the response of requests that fail validation,
// decoding or binding. By default they are written as an invalid-request error, with a field error per problem,
// see RequestFields
func WithRequestErrorHandler(h func(http.ResponseWriter, *http.Request, error)) Option {
	return func(o *options) {
		o.requestErrorHandler = h
	}
}

// WithErrorMapper converts the errors returned by handlers into the error response written for them, eg for errors
// which are not API errors (see the apierror package), or to observe them
// When it returns nil, the error is written as is: API errors with the status of their type, and any other error as
// a 500
func WithErrorMapper(m func(context.Context, error) *fxhttp.Error) Option {
	return func(o *options) {
		o.errorMapper = m
	}
}

// WithErrorTypes registers the error types of the service, which operations can list in x-errors, see
// ErrorsExtension. They must follow the rules of apierror.Validate, or NewHandler and Render fail
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
// error response with an error type it does not declare, with another status than the one of the error type, or with
// a field error whose code is not one of apierror.Codes
// Request validation failures and server errors (5xx) are always allowed
// Use it to fail tests, or to log violations in production
func WithContractViolationHandler(h func(r *http.Request, operationID string, e *fxhttp.Error)) Option {
	return func(o *options) {
		o.contractViolation = h
	}
}

func (o *options) catalog() (*errorCatalog, error) {
	return newErrorCatalog(o.errorTypes)
}

// writeResponseError writes the error returned by a handler, through the error mapper
func (o *options) writeResponseError(w http.ResponseWriter, r *http.Request, err error) {
	if o.errorMapper != nil {
		if e := o.errorMapper(r.Context(), err); e != nil {
			err = e
		}
	}
	fxhttp.WriteError(w, r, err)
}

func newOptions(opts []Option) *options {
	o := &options{requestErrorHandler: writeBadRequest}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// writeBadRequest writes err as an invalid-request error, with a field error per problem. The message of err is only
// kept as the cause: it can quote the values the client sent
func writeBadRequest(w http.ResponseWriter, r *http.Request, err error) {
	fxhttp.WriteError(w, r, &apierror.Error{Type: apierror.InvalidRequest, Fields: RequestFields(err), Cause: err})
}
