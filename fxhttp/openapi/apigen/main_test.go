package main

import (
	"os"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/oapi-codegen/oapi-codegen/v2/pkg/codegen"
	"github.com/stretchr/testify/require"
)

func TestGenerate(t *testing.T) {
	t.Run("Should match the generated code of the example", func(t *testing.T) {
		cfg := configuration{Configuration: codegen.Configuration{
			PackageName: "example",
			Generate:    codegen.GenerateOptions{Models: true, StdHTTPServer: true, Strict: true},
		}}
		cfg.Configuration = cfg.UpdateDefaults()
		server, err := generateServer("example/api.yaml", cfg)
		require.NoError(t, err)
		want, err := os.ReadFile("example/server.gen.go")
		require.NoError(t, err)
		require.Equal(t, string(want), string(server), "run go generate ./fxhttp/openapi/apigen/example")

		spec, err := openapi3.NewLoader().LoadFromFile("example/api.yaml")
		require.NoError(t, err)
		fields, err := generateFields(spec, "example", "api.yaml")
		require.NoError(t, err)
		want, err = os.ReadFile("example/fields.gen.go")
		require.NoError(t, err)
		require.Equal(t, string(want), string(fields), "run go generate ./fxhttp/openapi/apigen/example")
	})

	t.Run("Should refuse names Go can't tell apart", func(t *testing.T) {
		for name, schema := range map[string]string{
			"same Go name":       `{"type": "object", "properties": {"key-id": {"type": "string"}, "key_id": {"type": "string"}}}`,
			"name of the method": `{"oneOf": [{"type": "array", "items": {"type": "string"}}, {"type": "object", "properties": {"at": {"type": "string"}}}]}`,
		} {
			t.Run(name, func(t *testing.T) {
				spec, err := openapi3.NewLoader().LoadFromData([]byte(`{"openapi": "3.0.3", "info": {"title": "t", "version": "1"}, "paths": {"/x": {"post": {
					"requestBody": {"content": {"application/json": {"schema": ` + schema + `}}},
					"responses": {"204": {"description": "ok"}}}}}}`))
				require.NoError(t, err)
				_, err = generateFields(spec, "x", "api.yaml")
				require.Error(t, err)
			})
		}
	})
}
