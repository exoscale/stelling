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

type Type struct {
	// Slug identifies the type: clients receive it as the type "#/problems/<slug>", so it must be stable
	Slug string
	// Parent is the shared type a service type refines
	Parent *SharedType

	status int
	shared *SharedType // set on the types of SharedType

	Title string

	Description string
}

func (t *Type) ID() string {
	return "#/problems/" + t.Slug
}

func (t *Type) Status() int {
	if t.Parent != nil {
		return t.Parent.t.status
	}
	return t.status
}

// Shared returns the shared type t is or refines, nil for a type which is neither
func (t *Type) Shared() *SharedType {
	if t.Parent != nil {
		return t.Parent
	}
	return t.shared
}

func (t *Type) New(detail string) *Error {
	return t.Wrap(detail, nil)
}

// Newf returns an occurrence of the type whose detail is formatted like fmt.Sprintf
func (t *Type) Newf(format string, args ...any) *Error {
	return t.New(fmt.Sprintf(format, args...))
}

func (t *Type) Wrap(detail string, cause error) *Error {
	return &Error{Type: t, Detail: detail, Cause: cause}
}

func (t *Type) WithFields(fields ...Field) *Error {
	return &Error{Type: t, Fields: fields}
}

func New(t Type, detail string) *Error {
	return t.New(detail)
}

type SharedType struct {
	t Type
}

func newShared(slug string, status int, title string) *SharedType {
	s := &SharedType{t: Type{Slug: slug, status: status, Title: title}}
	s.t.shared = s
	return s
}

func (s *SharedType) Slug() string  { return s.t.Slug }
func (s *SharedType) ID() string    { return s.t.ID() }
func (s *SharedType) Status() int   { return s.t.status }
func (s *SharedType) Title() string { return s.t.Title }

func (s *SharedType) WithFields(first Field, more ...Field) *Error {
	return &Error{Type: &s.t, Fields: append([]Field{first}, more...)}
}

func InternalError(cause error) *Error {
	return &Error{Type: &Internal.t, Cause: cause}
}

func SharedTypes() []*Type {
	return slices.Clone(sharedTypes)
}

const slugSegment = `[a-z0-9]+(-[a-z0-9]+)*`

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
		if t.Parent == nil || t.Parent.t.shared != t.Parent {
			return fmt.Errorf("error type %q: its parent must be a shared type, eg apierror.Conflict", t.Slug)
		}
		if t.Title == "" {
			return fmt.Errorf("error type %q requires a title", t.Slug)
		}
	}
	return nil
}

type Code string

func (c Code) Known() bool {
	_, ok := codeMessages[c]
	return ok
}

func (c Code) Reason(name string) string {
	if name == "" {
		name = "the request"
	}
	return strings.ReplaceAll(codeMessages[c], "{name}", name)
}

func Codes() map[Code]string {
	return maps.Clone(codeMessages)
}

type Field struct {
	Name   string `json:"name,omitempty"`
	In     string `json:"in,omitempty"`
	Code   Code   `json:"code"`
	Reason string `json:"reason,omitempty"`
}

func F(name string, code Code) Field {
	return Field{Name: name, Code: code, Reason: code.Reason(name)}
}

func FIn(in, name string, code Code) Field {
	f := F(name, code)
	f.In = in
	return f
}

// FieldName names a field of a request, as listed by the vocabulary apigen generates from a specification
type FieldName struct {
	in, name string
}

// NewFieldName is meant for generated vocabularies: elsewhere, take the name from the vocabulary
func NewFieldName(in, name string) FieldName {
	return FieldName{in: in, name: name}
}

func (n FieldName) fieldName() FieldName { return n }

// Name returns the name as the client sends it, eg rules[0].action
func (n FieldName) Name() string { return n.name }

// FieldNamer is a FieldName, or a node of a vocabulary which embeds one
// Its method is unexported, so that only vocabularies can implement it and a misspelled name can't compile
type FieldNamer interface {
	fieldName() FieldName
}

// FieldOf returns a field error of the given code about the named field
func FieldOf(n FieldNamer, code Code) Field {
	name := n.fieldName()
	if name.in == "" {
		return F(name.name, code)
	}
	return FIn(name.in, name.name, code)
}

// Error is an occurrence of an error type
type Error struct {
	Type       *Type
	Detail     string
	Fields     []Field
	Extensions map[string]any
	Cause      error
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

func (e *Error) Unwrap() error {
	return e.Cause
}

func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok || e.Type == nil || t.Type == nil {
		return false
	}
	return e.Type.Slug == t.Type.Slug
}

func (e *Error) WithExtension(name string, value any) *Error {
	c := *e
	c.Extensions = make(map[string]any, len(e.Extensions)+1)
	for k, v := range e.Extensions {
		c.Extensions[k] = v
	}
	c.Extensions[name] = value
	return &c
}

func (e *Error) WithDetail(detail string) *Error {
	c := *e
	c.Detail = detail
	return &c
}

func (e *Error) WithCause(cause error) *Error {
	c := *e
	c.Cause = cause
	return &c
}

// WithFields returns a copy of e with the given field errors added
func (e *Error) WithFields(fields ...Field) *Error {
	c := *e
	c.Fields = append(slices.Clone(e.Fields), fields...)
	return &c
}

func From(err error) *Error {
	e, _ := errors.AsType[*Error](err)
	return e
}

func TypeOf(err error) *Type {
	if e := From(err); e != nil && e.Type != nil {
		return e.Type
	}
	return &Internal.t
}
