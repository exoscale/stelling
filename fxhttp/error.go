package fxhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/exoscale/stelling/fxhttp/apierror"
)

// ContentType is the media type of the error responses
const ContentType = "application/problem+json"

// typeBlank is the RFC 9457 default for status codes without a shared error type
const typeBlank = "about:blank"

// Error is the standard shape of an error response, following RFC 9457 Problem Details
// It can be returned from a handler to control the content of the response written by WriteError
type Error struct {
	// Status is the http status code of the response. Defaults to 500
	Status int
	// Type is a uri reference identifying the error type. Defaults to the shared type of the status, see apierror
	Type string
	// Title is a short human readable summary of the error type. Defaults to the one of the shared type of the status
	Title string
	// Detail is a human readable explanation specific to this occurrence of the problem. It is optional
	Detail string
	// Instance identifies the occurrence. WriteError sets it to the request id
	Instance string
	// Errors are the problems with the fields of the request
	Errors []apierror.Field
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

// MarshalJSON serializes the error with its extensions as top level members. Empty members are omitted
// It has a value receiver, so that both Error values and pointers are serialized in the wire format
func (e Error) MarshalJSON() ([]byte, error) {
	d := e.withDefaults()
	members := make(map[string]any, len(d.Extensions)+6)
	for k, v := range d.Extensions {
		members[k] = v
	}
	members["type"] = d.Type
	members["title"] = d.Title
	members["status"] = d.Status
	if d.Detail != "" {
		members["detail"] = d.Detail
	}
	if d.Instance != "" {
		members["instance"] = d.Instance
	}
	if len(d.Errors) > 0 {
		members["errors"] = d.Errors
	}
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
		case "instance":
			err = json.Unmarshal(v, &parsed.Instance)
		case "errors":
			// Services which do not use this format yet send other shapes: keep them as an extension rather than lose them
			var fields []apierror.Field
			if json.Unmarshal(v, &fields) == nil && !slices.ContainsFunc(fields, func(f apierror.Field) bool { return f.Code == "" }) {
				parsed.Errors = fields
				continue
			}
			fallthrough
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

// withDefaults returns a copy of e with its missing fields defaulted from its shared type, or from its status
func (e *Error) withDefaults() *Error {
	d := *e
	if d.Status == 0 {
		d.Status = http.StatusInternalServerError
	}
	for _, t := range apierror.SharedTypes() {
		if d.Type == t.ID() || (d.Type == "" && t.Status() == d.Status) {
			d.Type = t.ID()
			if d.Title == "" {
				d.Title = t.Title
			}
		}
	}
	if d.Type == "" {
		d.Type = typeBlank
	}
	if d.Title == "" {
		d.Title = http.StatusText(d.Status)
	}
	return &d
}

// ErrorFromAPIError converts p into its error response, with the status of its type
func ErrorFromAPIError(p *apierror.Error) *Error {
	t := p.Type
	if t == nil {
		t = apierror.Internal
	}
	return &Error{Status: t.Status(), Type: t.ID(), Title: t.Title, Detail: p.Detail, Errors: p.Fields, Extensions: p.Extensions, Cause: p}
}

// errorFrom converts any error into an *Error
// If err wraps an *Error, it is returned with its defaults applied
// If err wraps an *apierror.Error, it is converted with the status of its type
// Any other error results in a 500 without detail: its message can reveal internals, so it is only kept as the Cause
func errorFrom(err error) *Error {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.withDefaults()
	}
	if p := apierror.From(err); p != nil {
		return ErrorFromAPIError(p)
	}
	return (&Error{Status: http.StatusInternalServerError, Cause: err}).withDefaults()
}

// WriteError writes err to w as an RFC 9457 error response, with the request id as its instance
// If err wraps an *Error, it is written as is. If it wraps an *apierror.Error, it is written with the status of
// its type. Any other error is written as a 500 without detail
// Its signature matches the error handler hooks of common http routers and code generators,
// so it can be passed to them as is
// The written error is recorded on the request, see ContextWithErrorRecorder
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	e := errorFrom(err)
	if r != nil {
		if e.Instance == "" {
			e.Instance = RequestIDFromContext(r.Context())
		}
		recordError(r.Context(), e)
	}

	body, mErr := json.Marshal(e)
	if mErr != nil {
		// Extensions can contain values that can not be serialized: drop them rather than fail
		e.Extensions = nil
		body, _ = json.Marshal(e)
	}

	w.Header().Set("Content-Type", ContentType)
	w.Header().Set("Content-Language", "en")
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
