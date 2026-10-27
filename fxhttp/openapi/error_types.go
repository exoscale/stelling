package openapi

import (
	"cmp"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/exoscale/stelling/fxhttp"
	"github.com/exoscale/stelling/fxhttp/apierror"
	"github.com/getkin/kin-openapi/openapi3"
)

// ErrorsExtension is the operation extension listing the error types an operation can return
// Each entry is the slug of a shared type or of a type of the service, or an object with the slug and a description
// specific to the operation:
//
//	x-errors:
//	  - kms/key-not-found
//	  - type: kms/key-disabled
//	    description: The key is disabled, enable it first
//
// The rendered specification declares the matching error responses, see Spec.Render
const ErrorsExtension = "x-errors"

// ProblemsExtension is the member of the rendered specification listing every error type the service can return,
// shared and its own, and the codes of the field errors. public-api checks it against its table
const ProblemsExtension = "x-problems"

// The names of the schemas describing fxhttp.Error and its field errors in the rendered specification
const (
	problemSchemaName    = "Problem"
	fieldErrorSchemaName = "ProblemFieldError"
)

// documentedErrorType is an error type with the http status of its responses
type documentedErrorType struct {
	Slug        string
	Type        string
	Status      int
	Title       string
	Description string
	// Parent is the slug of the shared type a service type refines
	Parent string
}

// declaredErrorType is an error type declared by an operation
type declaredErrorType struct {
	documentedErrorType
	description string
}

type errorCatalog struct {
	types map[string]documentedErrorType
}

func newErrorCatalog(types []*apierror.Type) (*errorCatalog, error) {
	if err := apierror.Validate(types...); err != nil {
		return nil, err
	}
	c := &errorCatalog{types: map[string]documentedErrorType{}}
	for _, t := range append(apierror.SharedTypes(), types...) {
		p := documentedErrorType{Slug: t.Slug, Type: t.ID(), Status: t.Status(), Title: t.Title, Description: t.Description}
		if t.Parent != nil {
			p.Parent = t.Parent.Slug
		}
		c.types[t.Slug] = p
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
	add := func(slug, description string) error {
		t, ok := c.types[slug]
		if !ok {
			return fmt.Errorf("unknown error type %q", slug)
		}
		if seen[slug] {
			return fmt.Errorf("error type %q is declared twice", slug)
		}
		seen[slug] = true
		if description == "" {
			description = t.Description
		}
		declared = append(declared, declaredErrorType{documentedErrorType: t, description: description})
		return nil
	}

	// Requests which do not match the specification are rejected before reaching the handler
	if takesInput(item, op) {
		if err := add(apierror.InvalidRequest.Slug, "The request does not match the specification of the operation, see the errors member."); err != nil {
			return nil, true, err
		}
	}
	for _, entry := range entries {
		var err error
		switch e := entry.(type) {
		case string:
			err = add(e, "")
		case map[string]any:
			slug, _ := e["type"].(string)
			description, _ := e["description"].(string)
			err = add(slug, description)
		default:
			err = fmt.Errorf("%s entries must be an error type slug or an object with a type", ErrorsExtension)
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

// problems returns the x-problems member: every error type of the catalog, and the codes of the field errors
func (c *errorCatalog) problems() map[string]any {
	types := make(map[string]any, len(c.types))
	for slug, t := range c.types {
		entry := map[string]any{"status": t.Status, "title": t.Title}
		if t.Parent != "" {
			entry["parent"] = t.Parent
		}
		if t.Description != "" {
			entry["description"] = t.Description
		}
		types[slug] = entry
	}
	codes := map[string]any{}
	for code, message := range apierror.Codes() {
		codes[string(code)] = message
	}
	return map[string]any{"types": types, "codes": codes}
}

// errorExtension is a service specific member of the error responses
type errorExtension struct {
	name   string
	schema string
}

var kebabCase = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// problemSchemas describe fxhttp.Error, its field errors and the extensions of the service
func problemSchemas(doc *openapi3.T, extensions []errorExtension) (problem, fieldError *openapi3.Schema, err error) {
	fieldError = openapi3.NewObjectSchema()
	fieldError.Description = "A problem with one field of the request."
	fieldError.Required = []string{"code"}
	fieldError.AdditionalProperties = openapi3.AdditionalProperties{Has: new(true)}
	fieldError.Properties = openapi3.Schemas{
		"name":   openapi3.NewSchemaRef("", describe(openapi3.NewStringSchema(), "The path of the field, eg rules[0].action. Absent for a problem with the whole request.")),
		"in":     openapi3.NewSchemaRef("", describe(openapi3.NewStringSchema().WithEnum("body", "query", "path", "header", "cookie").WithDefault("body"), "Where the field is.")),
		"code":   openapi3.NewSchemaRef("", describe(openapi3.NewStringSchema(), "A stable token identifying the problem, see x-problems.")),
		"reason": openapi3.NewSchemaRef("", describe(openapi3.NewStringSchema(), "The default message of the code.")),
	}

	fieldErrors := openapi3.NewArraySchema()
	fieldErrors.Items = &openapi3.SchemaRef{Ref: "#/components/schemas/" + fieldErrorSchemaName, Value: fieldError}

	problem = openapi3.NewObjectSchema()
	problem.Description = "RFC 9457 problem details."
	problem.Required = []string{"type", "title", "status"}
	problem.AdditionalProperties = openapi3.AdditionalProperties{Has: new(true)}
	problem.Properties = openapi3.Schemas{
		"type":     openapi3.NewSchemaRef("", describe(openapi3.NewStringSchema().WithFormat("uri-reference"), "The problem type, see the description of the response for the possible values.")),
		"title":    openapi3.NewSchemaRef("", describe(openapi3.NewStringSchema(), "The summary of the problem type, the same for every occurrence.")),
		"status":   openapi3.NewSchemaRef("", describe(openapi3.NewIntegerSchema().WithMin(100).WithMax(599), "The http status code of the response.")),
		"detail":   openapi3.NewSchemaRef("", describe(openapi3.NewStringSchema(), "An explanation specific to this occurrence of the problem.")),
		"instance": openapi3.NewSchemaRef("", describe(openapi3.NewStringSchema().WithFormat("uri-reference"), "The request id.")),
		"errors":   openapi3.NewSchemaRef("", describe(fieldErrors, "The problems with the fields of the request.")),
	}

	for _, ext := range extensions {
		if _, ok := problem.Properties[ext.name]; ok || !kebabCase.MatchString(ext.name) {
			return nil, nil, fmt.Errorf("error extension %q: its name must be in kebab-case, and not one of the standard members", ext.name)
		}
		var component *openapi3.SchemaRef
		if doc.Components != nil {
			component = doc.Components.Schemas[ext.schema]
		}
		if component == nil {
			return nil, nil, fmt.Errorf("error extension %q: schema %q is not a component of the spec", ext.name, ext.schema)
		}
		problem.Properties[ext.name] = &openapi3.SchemaRef{Ref: "#/components/schemas/" + ext.schema, Value: component.Value}
	}
	return problem, fieldError, nil
}

func describe(s *openapi3.Schema, description string) *openapi3.Schema {
	s.Description = description
	return s
}

// renderErrorResponses replaces x-errors by the error responses of each operation, described by the Problem schema
// generated from fxhttp.Error, and lists the error types of the service in x-problems
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
	_, hasSchema := doc.Components.Schemas[problemSchemaName]
	if !usesErrors && !hasSchema {
		return nil
	}

	// The schemas are always the ones of fxhttp.Error, even when the spec describes them itself
	problem, fieldError, err := problemSchemas(doc, extensions)
	if err != nil {
		return err
	}
	if doc.Components.Schemas == nil {
		doc.Components.Schemas = openapi3.Schemas{}
	}
	doc.Components.Schemas[problemSchemaName] = openapi3.NewSchemaRef("", problem)
	doc.Components.Schemas[fieldErrorSchemaName] = openapi3.NewSchemaRef("", fieldError)
	schemaRef := &openapi3.SchemaRef{Ref: "#/components/schemas/" + problemSchemaName, Value: problem}
	if doc.Extensions == nil {
		doc.Extensions = map[string]any{}
	}
	doc.Extensions[ProblemsExtension] = catalog.problems()

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
			for _, status := range slices.Sorted(maps.Keys(byStatus)) {
				code := strconv.Itoa(status)
				if op.Responses.Value(code) != nil {
					return fmt.Errorf("%s %s (%s): the %s response is declared both in responses and in %s", method, path, op.OperationID, code, ErrorsExtension)
				}
				op.Responses.Set(code, &openapi3.ResponseRef{Value: errorResponse(byStatus[status], schemaRef)})
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
		line := fmt.Sprintf("- `%s` (%s)", p.Type, p.Title)
		if p.description != "" {
			line += ": " + p.description
		}
		lines = append(lines, line)
		examples[p.Slug] = &openapi3.ExampleRef{Value: &openapi3.Example{
			Summary: p.Title,
			Value:   map[string]any{"type": p.Type, "title": p.Title, "status": p.Status},
		}}
	}

	description := "Possible error types:\n\n" + strings.Join(lines, "\n")
	media := openapi3.NewMediaType().WithSchemaRef(schema)
	media.Examples = examples
	return &openapi3.Response{
		Description: &description,
		Content:     openapi3.Content{fxhttp.ContentType: media},
	}
}
