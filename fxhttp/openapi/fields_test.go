package openapi

import (
	"net/http"
	"testing"

	"github.com/exoscale/stelling/fxhttp/apierror"
	"github.com/stretchr/testify/require"
)

const fieldsSpec = `
openapi: 3.0.0
info: {title: rules, version: "1"}
servers:
  - url: /
paths:
  /rules:
    put:
      operationId: put-rules
      parameters:
        - {name: scope, in: query, required: true, schema: {type: string, enum: [read, write]}}
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              additionalProperties: false
              required: [coordinate, rules]
              properties:
                coordinate: {type: string}
                disk-size: {type: integer, maximum: 100}
                rules:
                  type: array
                  items:
                    type: object
                    properties:
                      action: {type: string, enum: [download, upload]}
      responses:
        '204': {description: Stored}
`

func TestRequestFields(t *testing.T) {
	h, err := NewHandler([]*Surface{{
		Name: "rules",
		Spec: mustLoadSpec(t, fieldsSpec, nil),
		Mount: func(MountOptions) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
		},
	}})
	require.NoError(t, err)

	t.Run("Should name every faulty field as the client sent it, without its value", func(t *testing.T) {
		rec := serve(t, h, http.MethodPut, "/rules?scope=admin", `{"disk-size": 512, "rules": [{"action": "delete"}], "colour": "red"}`)
		e := requireError(t, rec, http.StatusBadRequest)
		require.ElementsMatch(t, []apierror.Field{
			apierror.FIn("query", "scope", apierror.CodeUnknownValue),
			apierror.F("coordinate", apierror.CodeRequired),
			apierror.F("disk-size", apierror.CodeTooLarge),
			apierror.F("rules[0].action", apierror.CodeUnknownValue),
			apierror.F("colour", apierror.CodeUnknownField),
		}, e.Errors)
		require.NotContains(t, rec.Body.String(), "admin")
		require.NotContains(t, rec.Body.String(), "512")
	})

	t.Run("Should report a malformed body as a problem with the whole request", func(t *testing.T) {
		e := requireError(t, serve(t, h, http.MethodPut, "/rules?scope=read", `{`), http.StatusBadRequest)
		require.Equal(t, []apierror.Field{apierror.F("", apierror.CodeInvalidFormat)}, e.Errors)
	})
}
