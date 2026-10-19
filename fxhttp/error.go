package fxhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/exoscale/stelling/fxhttp/apierror"
)

// Default error types, used when an Error does not specify one
const (
	TypeBadRequest     = "#/http-problem-types/request-invalid-parameters"
	TypeInternalServer = "#/http-problem-types/internal-server-error"
	typeNotFound       = "#/http-problem-types/resource-not-found"
	typeConflict       = "#/http-problem-types/conflict"
	// typeBlank is the RFC 9457 default for status codes without a more specific error type
	typeBlank = "about:blank"
)

// Error is the standard shape of an error response, following RFC 9457 Problem Details
// It can be returned from a handler to control the content of the response written by WriteError
type Error struct {
	// Status is the http status code of the response. Defaults to 500
	Status int
	// Type is a uri reference identifying the error type. Defaults based on Status
	Type string
	// Title is a short human readable summary of the error type. Defaults to the status text
	Title string
	// Detail is a human readable explanation specific to this occurrence of the problem
	Detail string
	// Extensions are additional members serialized next to the standard ones
	// Extensions can not override the standard members
	Extensions map[string]any
	// Cause is the underlying error. It is never serialized
	Cause error
}

func (e *Error) Error() string {
	msg := e.Title
	if e.Detail != "" {
		msg = fmt.Sprintf("%s: %s", msg, e.Detail)
	}
	if e.Cause != nil {
		msg = fmt.Sprintf("%s: %v", msg, e.Cause)
	}
	return msg
}

func (e *Error) Unwrap() error {
	return e.Cause
}

// MarshalJSON serializes the error with its extensions as top level members
// It has a value receiver, so that both Error values and pointers are serialized in the wire format
func (e Error) MarshalJSON() ([]byte, error) {
	d := e.withDefaults()
	members := make(map[string]any, len(d.Extensions)+4)
	for k, v := range d.Extensions {
		members[k] = v
	}
	members["type"] = d.Type
	members["title"] = d.Title
	members["status"] = d.Status
	members["detail"] = d.Detail
	return json.Marshal(members)
}

// UnmarshalJSON parses an error response. Unknown members are stored in Extensions as json.RawMessage
func (e *Error) UnmarshalJSON(data []byte) error {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil {
		return err
	}
	parsed := Error{}
	for k, v := range members {
		var err error
		switch k {
		case "type":
			err = json.Unmarshal(v, &parsed.Type)
		case "title":
			err = json.Unmarshal(v, &parsed.Title)
		case "status":
			err = json.Unmarshal(v, &parsed.Status)
		case "detail":
			err = json.Unmarshal(v, &parsed.Detail)
		default:
			if parsed.Extensions == nil {
				parsed.Extensions = map[string]any{}
			}
			parsed.Extensions[k] = v
		}
		if err != nil {
			return fmt.Errorf("invalid error member %q: %w", k, err)
		}
	}
	*e = parsed
	return nil
}

// withDefaults returns a copy of e with its missing fields defaulted from its status
func (e *Error) withDefaults() *Error {
	d := *e
	if d.Status == 0 {
		d.Status = http.StatusInternalServerError
	}
	if d.Type == "" {
		d.Type = defaultType(d.Status)
	}
	if d.Title == "" {
		d.Title = http.StatusText(d.Status)
	}
	return &d
}

func defaultType(status int) string {
	switch status {
	case http.StatusBadRequest:
		return TypeBadRequest
	case http.StatusNotFound:
		return typeNotFound
	case http.StatusConflict:
		return typeConflict
	case http.StatusInternalServerError:
		return TypeInternalServer
	default:
		return typeBlank
	}
}

// DefaultStatus returns the http status of the API errors of category c
func DefaultStatus(c apierror.Category) int {
	switch c {
	case apierror.InvalidArgument:
		return http.StatusBadRequest
	case apierror.NotFound:
		return http.StatusNotFound
	case apierror.Conflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// ErrorFromAPIError converts p into the error response written with the given status
func ErrorFromAPIError(p *apierror.Error, status int) *Error {
	e := &Error{Status: status, Title: p.Title, Detail: p.Detail, Extensions: p.Extensions, Cause: p}
	if p.Type != nil {
		e.Type = p.Type.ID
	}
	return e.withDefaults()
}

// errorFrom converts any error into an *Error
// If err wraps an *Error, it is returned with its defaults applied
// If err wraps an *apierror.Error, it is converted with the DefaultStatus of its category
// Any other error results in a 500 with the error message as detail
func errorFrom(err error) *Error {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.withDefaults()
	}
	if p := apierror.From(err); p != nil {
		return ErrorFromAPIError(p, DefaultStatus(apierror.CategoryOf(p)))
	}
	return (&Error{Status: http.StatusInternalServerError, Detail: err.Error(), Cause: err}).withDefaults()
}

// WriteError writes err to w as a json error response
// If err wraps an *Error, it is written as is. If it wraps an *apierror.Error, it is written with the DefaultStatus
// of its category. Any other error is written as a 500 with the error message as detail
// Its signature matches the error handler hooks of common http routers and code generators,
// so it can be passed to them as is
// The written error is recorded on the request, see ContextWithErrorRecorder
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	e := errorFrom(err)
	if r != nil {
		recordError(r.Context(), e)
	}

	body, mErr := json.Marshal(e)
	if mErr != nil {
		// Extensions can contain values that can not be serialized: drop them rather than fail
		e.Extensions = nil
		body, _ = json.Marshal(e)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	_, _ = w.Write(body)
}

type errorRecorderContextKey struct{}

var errorRecorderCtxKey = &errorRecorderContextKey{}

type errorRecorder struct {
	err *Error
}

// ContextWithErrorRecorder returns a context in which the error response of a request can be recorded
// Observability middleware use it to report on the error after the request was handled
// If ctx already contains a recorder, it is returned as is
func ContextWithErrorRecorder(ctx context.Context) context.Context {
	if _, ok := ctx.Value(errorRecorderCtxKey).(*errorRecorder); ok {
		return ctx
	}
	return context.WithValue(ctx, errorRecorderCtxKey, &errorRecorder{})
}

// recordError records e as the error response of the current request
// It is a no-op when ctx was not prepared with ContextWithErrorRecorder
func recordError(ctx context.Context, e *Error) {
	if rec, ok := ctx.Value(errorRecorderCtxKey).(*errorRecorder); ok && e != nil {
		rec.err = e.withDefaults()
	}
}

// RecordedError returns the error response recorded for the current request, or nil
func RecordedError(ctx context.Context) *Error {
	if rec, ok := ctx.Value(errorRecorderCtxKey).(*errorRecorder); ok {
		return rec.err
	}
	return nil
}
