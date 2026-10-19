package openapi

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/exoscale/stelling/fxhttp"
	"github.com/exoscale/stelling/fxhttp/apierror"
	"github.com/getkin/kin-openapi/openapi3"
)

// ErrorsExtension is the operation extension listing the error types an operation can return
// Each entry is the id of a apierror.Type, without the leading "#/", or an object with the id and a description specific to the operation:
//
//	x-errors:
//	  - kms-key/not-found
//	  - type: kms-key/key-disabled
//	    description: The key is disabled, enable it first
//
// The rendered specification declares the matching error responses, see Spec.Render
const ErrorsExtension = "x-errors"

// errorResponseSchemaName is the name of the schema describing fxhttp.Error in the rendered specification
const errorResponseSchemaName = "error-response"

// requestValidationErrorType is declared for every operation taking input: requests which do not match the
// specification are rejected before reaching the handler. Register an error type with the same ID to customize it
var requestValidationErrorType = &apierror.Type{
	ID:          fxhttp.TypeBadRequest,
	Category:    apierror.InvalidArgument,
	Title:       "Bad Request",
	Description: "The request does not match the specification of the operation, eg a malformed parameter or body.",
}

// documentedErrorType is an error type with the http status of its responses
type documentedErrorType struct {
	Type        string
	Status      int
	Title       string
	Description string
}

// ID returns the id operations reference the error type with in x-errors
func (p documentedErrorType) ID() string {
	return strings.TrimPrefix(p.Type, "#/")
}

// declaredErrorType is an error type declared by an operation
type declaredErrorType struct {
	documentedErrorType
	description string
}

type errorCatalog struct {
	types map[string]documentedErrorType
}

func newErrorCatalog(types []*apierror.Type, status func(apierror.Category) int) (*errorCatalog, error) {
	c := &errorCatalog{types: map[string]documentedErrorType{}}
	for _, t := range append([]*apierror.Type{requestValidationErrorType}, types...) {
		if t == nil || t.ID == "" || t.Title == "" {
			return nil, fmt.Errorf("error types require an id and a title")
		}
		p := documentedErrorType{Type: t.ID, Status: status(t.Category), Title: t.Title, Description: t.Description}
		if p.Status < 400 || p.Status > 599 {
			return nil, fmt.Errorf("error type %q: its status %d is not an error status", t.ID, p.Status)
		}
		// Registered types override the default request validation error type
		c.types[p.ID()] = p
	}
	return c, nil
}

func takesInput(item *openapi3.PathItem, op *openapi3.Operation) bool {
	return len(item.Parameters) > 0 || len(op.Parameters) > 0 || op.RequestBody != nil
}

// declared returns the error types declared by op, and whether op uses x-errors at all
func (c *errorCatalog) declared(item *openapi3.PathItem, op *openapi3.Operation) ([]declaredErrorType, bool, error) {
	raw, ok := op.Extensions[ErrorsExtension]
	if !ok {
		return nil, false, nil
	}
	entries, ok := raw.([]any)
	if !ok {
		return nil, true, fmt.Errorf("%s must be a list", ErrorsExtension)
	}

	var declared []declaredErrorType
	seen := map[string]bool{}
	add := func(id, description string) error {
		t, ok := c.types[strings.TrimPrefix(id, "#/")]
		if !ok {
			return fmt.Errorf("unknown error type %q", id)
		}
		if seen[t.ID()] {
			return fmt.Errorf("error type %q is declared twice", id)
		}
		seen[t.ID()] = true
		if description == "" {
			description = t.Description
		}
		declared = append(declared, declaredErrorType{documentedErrorType: t, description: description})
		return nil
	}

	if takesInput(item, op) {
		if err := add(strings.TrimPrefix(requestValidationErrorType.ID, "#/"), ""); err != nil {
			return nil, true, err
		}
	}
	for _, entry := range entries {
		var err error
		switch e := entry.(type) {
		case string:
			err = add(e, "")
		case map[string]any:
			id, _ := e["type"].(string)
			description, _ := e["description"].(string)
			err = add(id, description)
		default:
			err = fmt.Errorf("%s entries must be an error type id or an object with a type", ErrorsExtension)
		}
		if err != nil {
			return nil, true, err
		}
	}
	return declared, true, nil
}

// validate asserts every error type referenced in doc is registered
func (c *errorCatalog) validate(doc *openapi3.T) error {
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			if _, _, err := c.declared(item, op); err != nil {
				return fmt.Errorf("%s %s (%s): %w", method, path, op.OperationID, err)
			}
		}
	}
	return nil
}

// errorExtension is a service specific member of the error responses
type errorExtension struct {
	name   string
	schema string
}

// errorResponseSchema describes fxhttp.Error and the extensions of the service
func errorResponseSchema(doc *openapi3.T, extensions []errorExtension) (*openapi3.Schema, error) {
	s := openapi3.NewObjectSchema()
	s.Description = "RFC 9457 Problem Details error response"
	s.Required = []string{"type", "title", "status", "detail"}
	s.Properties = openapi3.Schemas{
		"type":   openapi3.NewSchemaRef("", describe(openapi3.NewStringSchema().WithFormat("uri-reference"), "A URI reference identifying the error type, see the description of the response for the possible values.")),
		"title":  openapi3.NewSchemaRef("", describe(openapi3.NewStringSchema(), "A short summary of the error type.")),
		"status": openapi3.NewSchemaRef("", describe(openapi3.NewIntegerSchema().WithMin(100).WithMax(599), "The http status code of the response.")),
		"detail": openapi3.NewSchemaRef("", describe(openapi3.NewStringSchema(), "A human readable explanation specific to this occurrence of the error.")),
	}

	for _, ext := range extensions {
		if _, ok := s.Properties[ext.name]; ok {
			return nil, fmt.Errorf("error extension %q can not override a standard member", ext.name)
		}
		var component *openapi3.SchemaRef
		if doc.Components != nil {
			component = doc.Components.Schemas[ext.schema]
		}
		if component == nil {
			return nil, fmt.Errorf("error extension %q: schema %q is not a component of the spec", ext.name, ext.schema)
		}
		s.Properties[ext.name] = &openapi3.SchemaRef{Ref: "#/components/schemas/" + ext.schema, Value: component.Value}
	}
	return s, nil
}

func describe(s *openapi3.Schema, description string) *openapi3.Schema {
	s.Description = description
	return s
}

// renderErrorResponses replaces x-errors by the error responses of each operation, described by the
// error-response schema generated from fxhttp.Error
func renderErrorResponses(doc *openapi3.T, catalog *errorCatalog, extensions []errorExtension) error {
	usesErrors := false
	for _, item := range doc.Paths.Map() {
		for _, op := range item.Operations() {
			if _, ok := op.Extensions[ErrorsExtension]; ok {
				usesErrors = true
			}
		}
	}
	if doc.Components == nil {
		doc.Components = &openapi3.Components{}
	}
	_, hasSchema := doc.Components.Schemas[errorResponseSchemaName]
	if !usesErrors && !hasSchema {
		return nil
	}

	// The schema is always the one of fxhttp.Error, even when the spec describes it itself
	schema, err := errorResponseSchema(doc, extensions)
	if err != nil {
		return err
	}
	if doc.Components.Schemas == nil {
		doc.Components.Schemas = openapi3.Schemas{}
	}
	doc.Components.Schemas[errorResponseSchemaName] = openapi3.NewSchemaRef("", schema)
	schemaRef := &openapi3.SchemaRef{Ref: "#/components/schemas/" + errorResponseSchemaName, Value: schema}

	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			declared, ok, err := catalog.declared(item, op)
			if err != nil {
				return fmt.Errorf("%s %s (%s): %w", method, path, op.OperationID, err)
			}
			if !ok {
				continue
			}
			delete(op.Extensions, ErrorsExtension)

			byStatus := map[int][]declaredErrorType{}
			for _, p := range declared {
				byStatus[p.Status] = append(byStatus[p.Status], p)
			}
			for status, group := range byStatus {
				code := strconv.Itoa(status)
				if op.Responses.Value(code) != nil {
					return fmt.Errorf("%s %s (%s): the %s response is declared both in responses and in %s", method, path, op.OperationID, code, ErrorsExtension)
				}
				op.Responses.Set(code, &openapi3.ResponseRef{Value: errorResponse(group, schemaRef)})
			}
		}
	}
	return nil
}

func errorResponse(declared []declaredErrorType, schema *openapi3.SchemaRef) *openapi3.Response {
	slices.SortFunc(declared, func(a, b declaredErrorType) int { return cmp.Compare(a.Type, b.Type) })

	lines := make([]string, 0, len(declared))
	examples := openapi3.Examples{}
	for _, p := range declared {
		lines = append(lines, fmt.Sprintf("- `%s` (%s): %s", p.Type, p.Title, p.description))
		examples[p.ID()] = &openapi3.ExampleRef{Value: &openapi3.Example{
			Summary: p.Title,
			Value: map[string]any{
				"type":   p.Type,
				"title":  p.Title,
				"status": p.Status,
				"detail": p.description,
			},
		}}
	}

	description := "Possible error types:\n\n" + strings.Join(lines, "\n")
	media := openapi3.NewMediaType().WithSchemaRef(schema)
	media.Examples = examples
	return &openapi3.Response{
		Description: &description,
		Content:     openapi3.Content{"application/json": media},
	}
}
