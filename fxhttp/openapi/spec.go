package openapi

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

type Spec struct {
	doc  *openapi3.T
	data []byte
	refs map[string][]byte
}

// LoadSpec loads the OpenAPI specification in data
// External references are resolved from refs, which maps a path suffix to the content of the referenced file,
// eg: {"publicapi/api.yaml": publicSpec} for `$ref: '../publicapi/api.yaml#/components/schemas/error-response'`
// This allows loading specifications embedded in the binary, without access to the filesystem
func LoadSpec(data []byte, refs map[string][]byte) (*Spec, error) {
	s := &Spec{data: data, refs: refs}
	doc, err := s.load()
	if err != nil {
		return nil, err
	}
	if err := doc.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("invalid OpenAPI spec: %w", err)
	}
	s.doc = doc
	return s, nil
}

func (s *Spec) load() (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	if len(s.refs) > 0 {
		loader.IsExternalRefsAllowed = true
		loader.ReadFromURIFunc = func(_ *openapi3.Loader, uri *url.URL) ([]byte, error) {
			for suffix, data := range s.refs {
				if strings.HasSuffix(uri.Path, suffix) {
					return data, nil
				}
			}
			return nil, fmt.Errorf("unexpected external ref: %s", uri.String())
		}
	}
	return loader.LoadFromData(s.data)
}

// Document returns the loaded OpenAPI document
// It must not be modified
func (s *Spec) Document() *openapi3.T {
	return s.doc
}

// selfContainedDocument returns a fresh copy of the document in which every external reference is inlined
func (s *Spec) selfContainedDocument() (*openapi3.T, error) {
	// Internalizing modifies the document, so we work on a fresh copy
	doc, err := s.load()
	if err != nil {
		return nil, err
	}

	resolver := newComponentNameResolver(doc)
	doc.InternalizeRefs(context.Background(), resolver.resolve)
	if err := resolver.err(); err != nil {
		return nil, err
	}
	return doc, nil
}

// componentNameResolver names internalized components after their name in their own document
type componentNameResolver struct {
	sources map[string]string
	errs    []error
}

func newComponentNameResolver(doc *openapi3.T) *componentNameResolver {
	r := &componentNameResolver{sources: map[string]string{}}
	if c := doc.Components; c != nil {
		// Components of the root document keep their name
		addRootComponents(r, "schemas", c.Schemas)
		addRootComponents(r, "parameters", c.Parameters)
		addRootComponents(r, "headers", c.Headers)
		addRootComponents(r, "requestBodies", c.RequestBodies)
		addRootComponents(r, "responses", c.Responses)
		addRootComponents(r, "securitySchemes", c.SecuritySchemes)
		addRootComponents(r, "examples", c.Examples)
		addRootComponents(r, "links", c.Links)
		addRootComponents(r, "callbacks", c.Callbacks)
	}
	return r
}

func addRootComponents[V any](r *componentNameResolver, collection string, components map[string]V) {
	for name := range components {
		r.sources[path.Join(collection, name)] = ""
	}
}

func (r *componentNameResolver) resolve(doc *openapi3.T, ref openapi3.ComponentRef) string {
	if name, found := openapi3.ReferencesComponentInRootDocument(doc, ref); found {
		return path.Base(name)
	}

	refPath := ref.RefPath()
	name := path.Base(refPath.Fragment)
	key := path.Join(ref.CollectionName(), name)
	source := refPath.String()

	if existing, ok := r.sources[key]; ok && existing != source {
		if existing == "" {
			existing = "the root document"
		}
		r.errs = append(r.errs, fmt.Errorf("component %q from %s collides with the one from %s", key, source, existing))
	} else {
		r.sources[key] = source
	}
	return name
}

func (r *componentNameResolver) err() error {
	return errors.Join(r.errs...)
}
