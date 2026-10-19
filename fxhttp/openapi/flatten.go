package openapi

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/getkin/kin-openapi/openapi3"
)

// schemaVisitor visits every schema of a document once
type schemaVisitor struct {
	visited map[*openapi3.Schema]bool
	visit   func(*openapi3.Schema) error
}

func walkSchemas(doc *openapi3.T, visit func(*openapi3.Schema) error) error {
	v := &schemaVisitor{visited: map[*openapi3.Schema]bool{}, visit: visit}

	if doc.Components != nil {
		for _, s := range doc.Components.Schemas {
			if err := v.schemaRef(s); err != nil {
				return err
			}
		}
		for _, p := range doc.Components.Parameters {
			if err := v.parameter(p); err != nil {
				return err
			}
		}
		for _, h := range doc.Components.Headers {
			if err := v.header(h); err != nil {
				return err
			}
		}
		for _, b := range doc.Components.RequestBodies {
			if b != nil && b.Value != nil {
				if err := v.content(b.Value.Content); err != nil {
					return err
				}
			}
		}
		for _, r := range doc.Components.Responses {
			if err := v.response(r); err != nil {
				return err
			}
		}
	}

	for _, item := range doc.Paths.Map() {
		for _, p := range item.Parameters {
			if err := v.parameter(p); err != nil {
				return err
			}
		}
		for _, op := range item.Operations() {
			for _, p := range op.Parameters {
				if err := v.parameter(p); err != nil {
					return err
				}
			}
			if op.RequestBody != nil && op.RequestBody.Value != nil {
				if err := v.content(op.RequestBody.Value.Content); err != nil {
					return err
				}
			}
			if op.Responses != nil {
				for _, r := range op.Responses.Map() {
					if err := v.response(r); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (v *schemaVisitor) parameter(p *openapi3.ParameterRef) error {
	if p == nil || p.Value == nil {
		return nil
	}
	if err := v.schemaRef(p.Value.Schema); err != nil {
		return err
	}
	return v.content(p.Value.Content)
}

func (v *schemaVisitor) header(h *openapi3.HeaderRef) error {
	if h == nil || h.Value == nil {
		return nil
	}
	return v.parameter(&openapi3.ParameterRef{Value: &h.Value.Parameter})
}

func (v *schemaVisitor) response(r *openapi3.ResponseRef) error {
	if r == nil || r.Value == nil {
		return nil
	}
	for _, h := range r.Value.Headers {
		if err := v.header(h); err != nil {
			return err
		}
	}
	return v.content(r.Value.Content)
}

func (v *schemaVisitor) content(c openapi3.Content) error {
	for _, media := range c {
		if media != nil {
			if err := v.schemaRef(media.Schema); err != nil {
				return err
			}
		}
	}
	return nil
}

func (v *schemaVisitor) schemaRef(ref *openapi3.SchemaRef) error {
	if ref == nil || ref.Value == nil || v.visited[ref.Value] {
		return nil
	}
	s := ref.Value
	v.visited[s] = true

	// Visit the members first, so the schema is visited after the schemas it is composed of
	for _, members := range []openapi3.SchemaRefs{s.AllOf, s.OneOf, s.AnyOf} {
		for _, m := range members {
			if err := v.schemaRef(m); err != nil {
				return err
			}
		}
	}
	for _, p := range s.Properties {
		if err := v.schemaRef(p); err != nil {
			return err
		}
	}
	if err := v.schemaRef(s.Items); err != nil {
		return err
	}
	if err := v.schemaRef(s.Not); err != nil {
		return err
	}
	if err := v.schemaRef(s.AdditionalProperties.Schema); err != nil {
		return err
	}
	return v.visit(s)
}

// flattenAllOf merges the members of every allOf into the schema declaring it
// Only object schemas can be merged: the members contribute their properties and required properties
// Merging fails rather than producing a schema which accepts different documents
func flattenAllOf(doc *openapi3.T) error {
	return walkSchemas(doc, func(s *openapi3.Schema) error {
		if len(s.AllOf) == 0 {
			return nil
		}
		members := s.AllOf
		s.AllOf = nil
		for i, m := range members {
			if err := mergeObjectSchema(s, m.Value); err != nil {
				name := m.Ref
				if name == "" {
					name = fmt.Sprintf("member %d", i)
				}
				return fmt.Errorf("flatten allOf (%s): %w", name, err)
			}
		}
		return nil
	})
}

func mergeObjectSchema(dst, src *openapi3.Schema) error {
	if src == nil {
		return nil
	}
	if src.Type != nil && !src.Type.Is(openapi3.TypeObject) {
		return fmt.Errorf("only object schemas can be merged, got %v", src.Type.Slice())
	}
	if dst.Type != nil && !dst.Type.Is(openapi3.TypeObject) {
		return fmt.Errorf("only object schemas can be merged into, got %v", dst.Type.Slice())
	}
	if len(src.OneOf) > 0 || len(src.AnyOf) > 0 || src.Not != nil || src.Discriminator != nil {
		return fmt.Errorf("members with oneOf, anyOf, not or a discriminator can not be merged")
	}
	if len(src.Enum) > 0 || src.Items != nil || src.Format != "" {
		return fmt.Errorf("members with enum, items or a format can not be merged")
	}

	if dst.Type == nil {
		dst.Type = &openapi3.Types{openapi3.TypeObject}
	}
	if dst.Description == "" {
		dst.Description = src.Description
	}
	if dst.Title == "" {
		dst.Title = src.Title
	}

	if len(src.Properties) > 0 && dst.Properties == nil {
		dst.Properties = openapi3.Schemas{}
	}
	for name, prop := range src.Properties {
		existing, ok := dst.Properties[name]
		if ok && existing != prop {
			same, err := sameJSON(existing, prop)
			if err != nil {
				return err
			}
			if !same {
				return fmt.Errorf("property %q is defined differently by several members", name)
			}
			continue
		}
		dst.Properties[name] = prop
	}

	for _, name := range src.Required {
		if !slices.Contains(dst.Required, name) {
			dst.Required = append(dst.Required, name)
		}
	}

	if src.AdditionalProperties.Has != nil || src.AdditionalProperties.Schema != nil {
		if dst.AdditionalProperties.Has != nil || dst.AdditionalProperties.Schema != nil {
			same, err := sameJSON(src.AdditionalProperties, dst.AdditionalProperties)
			if err != nil {
				return err
			}
			if !same {
				return fmt.Errorf("additionalProperties is defined differently by several members")
			}
		}
		dst.AdditionalProperties = src.AdditionalProperties
	}
	return nil
}

func sameJSON(a, b any) (bool, error) {
	ja, err := json.Marshal(a)
	if err != nil {
		return false, err
	}
	jb, err := json.Marshal(b)
	if err != nil {
		return false, err
	}
	return string(ja) == string(jb), nil
}

// assertNoAllOf fails if any schema of doc still uses allOf
func assertNoAllOf(doc *openapi3.T) error {
	return walkSchemas(doc, func(s *openapi3.Schema) error {
		if len(s.AllOf) > 0 {
			return fmt.Errorf("the rendered spec must not contain allOf")
		}
		return nil
	})
}
