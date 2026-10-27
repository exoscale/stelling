// Package apierror defines the errors a service returns to its clients, independently of the transport
//
// The types shared by every service, and the codes of the field errors, are generated from problems.json, a copy of
// the table public-api publishes in its specification. A service declares its own types as refinements of a shared
// one, and returns occurrences of them from any layer. fxhttp writes them as RFC 9457 Problem Details, and the
// openapi package documents them in the specifications
// The package has no dependencies, so it can be used in the domain layer of a service
package apierror

//go:generate go run ./gen problems.json problems_gen.go

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// Type is a kind of error a service returns to its clients
type Type struct {
	// Slug identifies the type: clients receive it as the type "#/problems/<slug>", so it must be stable
	// Shared types have a single segment, eg "not-found". Service types are under the namespace of the service,
	// eg "kms/key-disabled"
	Slug string
	// Parent is the shared type a service type refines. It gives the status of its errors, and their meaning for the
	// clients which do not know the service type. It is nil for shared types
	Parent *Type
	// status is the http status of the errors of a shared type
	status int
	// Title is the summary of the type, the same for every occurrence
	Title string
	// Description explains when the error occurs, for the readers of the API documentation
	Description string
}

// ID returns the RFC 9457 type of the errors, eg "#/problems/kms/key-disabled"
func (t *Type) ID() string {
	return "#/problems/" + t.Slug
}

// Status returns the http status of the errors of the type, the one of its parent for a service type
func (t *Type) Status() int {
	if t.Parent != nil {
		return t.Parent.status
	}
	return t.status
}

// IsClient reports whether the errors of the type are caused by the request, rather than by the service
func (t *Type) IsClient() bool {
	return t.Status() < 500
}

// New returns an occurrence of the error type
func (t *Type) New(detail string) *Error {
	return t.Wrap(detail, nil)
}

// Wrap returns an occurrence of the error type, caused by cause
func (t *Type) Wrap(detail string, cause error) *Error {
	return &Error{Type: t, Detail: detail, Cause: cause}
}

// WithFields returns an occurrence of the error type about the given fields of the request
func (t *Type) WithFields(fields ...Field) *Error {
	return &Error{Type: t, Fields: fields}
}

// New declares an error type and returns its sentinel, an occurrence with the given detail, which can be empty
// Sentinels can be returned as is, and are matched with errors.Is against any occurrence of their type
func New(t Type, detail string) *Error {
	return t.New(detail)
}

// SharedTypes returns the types shared by every service
func SharedTypes() []*Type {
	return slices.Clone(sharedTypes)
}

var slugSegment = `[a-z0-9]+(-[a-z0-9]+)*`
var serviceSlug = regexp.MustCompile(`^` + slugSegment + `/` + slugSegment + `$`)

// Validate checks the types a service declares: their slug is under a single namespace, eg "kms/key-disabled",
// they refine a shared type, and they have a title
func Validate(types ...*Type) error {
	namespace := ""
	seen := map[string]bool{}
	for _, t := range types {
		if t == nil {
			return errors.New("error types can not be nil")
		}
		if !serviceSlug.MatchString(t.Slug) {
			return fmt.Errorf("error type %q: the slug must be <service>/<name>, in kebab-case", t.Slug)
		}
		ns, _, _ := strings.Cut(t.Slug, "/")
		if namespace == "" {
			namespace = ns
		} else if ns != namespace {
			return fmt.Errorf("error type %q: every type of the service must be under the namespace %q", t.Slug, namespace)
		}
		if seen[t.Slug] {
			return fmt.Errorf("error type %q is declared twice", t.Slug)
		}
		seen[t.Slug] = true
		if !slices.Contains(sharedTypes, t.Parent) {
			return fmt.Errorf("error type %q: its parent must be a shared type, eg apierror.Conflict", t.Slug)
		}
		if t.Title == "" {
			return fmt.Errorf("error type %q requires a title", t.Slug)
		}
	}
	return nil
}

// Code identifies the problem of a field error, eg CodeRequired. Clients translate it: the codes are a closed list
type Code string

// Known reports whether c is one of the codes of problems.json
func (c Code) Known() bool {
	_, ok := codeMessages[c]
	return ok
}

// Reason returns the default message of the code for the given field, eg "coordinate is required"
func (c Code) Reason(name string) string {
	if name == "" {
		name = "the request"
	}
	return strings.ReplaceAll(codeMessages[c], "{name}", name)
}

// Codes returns the message of every code, with {name} standing for the name of the field
func Codes() map[Code]string {
	return maps.Clone(codeMessages)
}

// Field is a problem with one field of the request
type Field struct {
	// Name is the path of the field, built from the names the client sent, eg "rules[0].action"
	// It is empty for a problem with the whole request
	Name string `json:"name,omitempty"`
	// In is where the field is: query, path, header or cookie. It is empty for the body
	In string `json:"in,omitempty"`
	// Code identifies the problem
	Code Code `json:"code"`
	// Reason is the default message of the code, see Code.Reason
	Reason string `json:"reason,omitempty"`
}

// F returns the field error of the body field name
func F(name string, code Code) Field {
	return Field{Name: name, Code: code, Reason: code.Reason(name)}
}

// FIn returns the field error of a field outside of the body, eg FIn("query", "scope", CodeUnknownValue)
func FIn(in, name string, code Code) Field {
	f := F(name, code)
	f.In = in
	return f
}

// Error is an occurrence of an error type
type Error struct {
	// Type is the error type of the error
	Type *Type
	// Detail explains this occurrence of the error. It is optional: the title of the type is the same for every one
	Detail string
	// Fields are the problems with the fields of the request
	Fields []Field
	// Extensions are additional members returned to clients, which can not override the standard ones
	// Their values are serialized as is: use types with the wire format of the member
	Extensions map[string]any
	// Cause is the underlying error. It is never returned to clients
	Cause error
}

func (e *Error) Error() string {
	msg := "error"
	if e.Type != nil {
		msg = e.Type.Title
	}
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	for _, f := range e.Fields {
		msg += ": " + f.Reason
	}
	if e.Cause != nil {
		msg += fmt.Sprintf(": %v", e.Cause)
	}
	return msg
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
	return e.Type.Slug == t.Type.Slug
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

// TypeOf returns the error type of the first API error in the chain of err, or Internal
// Errors which are not API errors are internal
func TypeOf(err error) *Type {
	if e := From(err); e != nil && e.Type != nil {
		return e.Type
	}
	return Internal
}
