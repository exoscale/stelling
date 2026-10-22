// Package apierror defines the errors a service returns to its clients, independently of the transport
//
// A service declares its error types once, as sentinels, and returns occurrences of them from any layer.
// The transport modules convert them to their wire format: fxhttp writes them as RFC 9457 Problem Details,
// and the openapi package documents them in the specifications
// The package has no dependencies, so it can be used in the domain layer of a service
package apierror

import (
	"errors"
	"fmt"
)

// Category classifies error types by the kind of failure, like gRPC status codes
// Transports derive their status from it, eg an http status code
type Category int

const (
	// Internal is an unexpected failure of the service. It is the zero value
	Internal Category = iota
	// InvalidArgument is a request which can not succeed as is
	InvalidArgument
	// NotFound is a request for a resource which does not exist
	NotFound
	// Conflict is a request which conflicts with the current state of a resource
	Conflict
)

// IsClient reports whether errors of the category are caused by the request, rather than by the service
func (c Category) IsClient() bool {
	switch c {
	case InvalidArgument, NotFound, Conflict:
		return true
	default:
		return false
	}
}

// Type is a kind of error a service returns to its clients
type Type struct {
	// ID is the RFC 9457 type of the errors, eg "#/kms-key/not-found"
	// It identifies the error type for clients: it must be stable
	ID string
	// Category classifies the error type
	Category Category
	// Title is the short summary of the error type
	Title string
	// Description explains when the error occurs, for the readers of the API documentation
	Description string
}

// New returns an occurrence of the error type
func (t *Type) New(detail string) *Error {
	return t.Wrap(detail, nil)
}

// Wrap returns an occurrence of the error type, caused by cause
func (t *Type) Wrap(detail string, cause error) *Error {
	return &Error{Type: t, Title: t.Title, Detail: detail, Cause: cause}
}

// New declares an error type and returns its sentinel, an occurrence with a default detail
// Sentinels can be returned as is, and are matched with errors.Is against any occurrence of their type
func New(t Type, detail string) *Error {
	return t.New(detail)
}

// Error is an occurrence of an error type
type Error struct {
	// Type is the error type of the error
	Type *Type
	// Title is the short summary of the error. It defaults to the title of the type
	Title string
	// Detail explains this occurrence of the error
	Detail string
	// Extensions are additional members returned to clients, which can not override the standard ones
	// Their values are serialized as is: use types with the wire format of the member
	Extensions map[string]any
	// Cause is the underlying error. It is never returned to clients
	Cause error
}

func (e *Error) Error() string {
	switch {
	case e.Detail != "" && e.Cause != nil:
		return fmt.Sprintf("%s: %s: %v", e.Title, e.Detail, e.Cause)
	case e.Detail != "":
		return fmt.Sprintf("%s: %s", e.Title, e.Detail)
	case e.Cause != nil:
		return fmt.Sprintf("%s: %v", e.Title, e.Cause)
	case e.Title != "":
		return e.Title
	default:
		return "error"
	}
}

// Unwrap returns the cause of the error
func (e *Error) Unwrap() error {
	return e.Cause
}

// Is reports whether target is an occurrence of the same error type
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok || e.Type == nil || t.Type == nil {
		return false
	}
	return e.Type.ID == t.Type.ID && e.Type.Category == t.Type.Category
}

// WithExtension returns a copy of e with the given extension member
func (e *Error) WithExtension(name string, value any) *Error {
	c := *e
	c.Extensions = make(map[string]any, len(e.Extensions)+1)
	for k, v := range e.Extensions {
		c.Extensions[k] = v
	}
	c.Extensions[name] = value
	return &c
}

// From returns the first API error in the chain of err, or nil
func From(err error) *Error {
	e, ok := errors.AsType[*Error](err)
	if !ok {
		return nil
	}
	return e
}

// TypeOf returns the error type of the first API error in the chain of err, or nil
func TypeOf(err error) *Type {
	if e := From(err); e != nil {
		return e.Type
	}
	return nil
}

// CategoryOf returns the category of the first API error in the chain of err
// Errors which are not API errors are internal
func CategoryOf(err error) Category {
	if t := TypeOf(err); t != nil {
		return t.Category
	}
	return Internal
}
