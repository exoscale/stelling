package openapi

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

// Render returns the specification as consumed by clients and documentation tools:
//   - external references are inlined, under the name the components have in their own document, and the
//     references are rewritten to point to them
//   - the error responses of the operations listing their error types in x-errors are declared,
//     with the error-response schema of fxhttp.Error and an example per error type, see WithErrorTypes
//
// The result is validated, and is guaranteed to only contain local references
// The served specification of a surface is the rendered one
func (s *Spec) Render(opts ...Option) ([]byte, error) {
	o := newOptions(opts)
	catalog, err := o.catalog()
	if err != nil {
		return nil, err
	}

	doc, err := s.selfContainedDocument()
	if err != nil {
		return nil, err
	}
	if err := renderErrorResponses(doc, catalog, o.errorExtensions); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("marshal OpenAPI spec: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("marshal OpenAPI spec: %w", err)
	}
	data := buf.Bytes()

	// The result must stand on its own: no external reference, and valid, examples included
	rendered, err := openapi3.NewLoader().LoadFromData(data)
	if err != nil {
		return nil, fmt.Errorf("rendered spec is not self contained: %w", err)
	}
	if err := rendered.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("rendered spec is invalid: %w", err)
	}

	return data, nil
}

// RenderTarget is a file in which a rendered specification is written
type RenderTarget struct {
	// Path of the file, relative to the working directory
	Path string
	Spec *Spec
	// Options are applied after the shared options, see Surface.SpecOptions
	Options []Option
}

// RenderFiles renders every target, with the same options as the served specifications
// With check, the files are not written: an error is returned if any of them is not up to date
func RenderFiles(targets []RenderTarget, check bool, opts ...Option) error {
	var errs []error
	for _, t := range targets {
		data, err := t.Spec.Render(append(slices.Clone(opts), t.Options...)...)
		if err != nil {
			errs = append(errs, fmt.Errorf("render %s: %w", t.Path, err))
			continue
		}
		if check {
			current, err := os.ReadFile(t.Path)
			if err != nil || !bytes.Equal(current, data) {
				errs = append(errs, fmt.Errorf("%s is not up to date", t.Path))
			}
			continue
		}
		if err := os.WriteFile(t.Path, data, 0o644); err != nil {
			errs = append(errs, fmt.Errorf("write %s: %w", t.Path, err))
		}
	}
	return errors.Join(errs...)
}

// RenderMain is the entrypoint of a command rendering the specifications of a service, typically invoked
// by go generate:
//
//	//go:generate go run ./cmd/openapi-render
//
//	func main() {
//		openapi.RenderMain([]openapi.RenderTarget{{Path: "api/public/api.rendered.yaml", Spec: publicSpec}}, opts...)
//	}
//
// Pass the -check flag to verify the files are up to date instead of writing them
func RenderMain(targets []RenderTarget, opts ...Option) {
	check := flag.Bool("check", false, "verify the rendered specs are up to date instead of writing them")
	flag.Parse()

	if err := RenderFiles(targets, *check, opts...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
