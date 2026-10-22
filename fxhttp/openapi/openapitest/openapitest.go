package openapitest

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/exoscale/stelling/fxhttp"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

// errorResponseSchemaName is the name of the error response schema of the rendered specifications
// It duplicates the one of the openapi package, which can not be imported: the tests of openapi use this package
const errorResponseSchemaName = "error-response"

// ErrorResponseSchema returns the error response schema declared by the 400 response of op, or nil
func ErrorResponseSchema(op *openapi3.Operation) *openapi3.SchemaRef {
	if op == nil || op.Responses == nil {
		return nil
	}
	resp := op.Responses.Status(http.StatusBadRequest)
	if resp == nil || resp.Value == nil {
		return nil
	}
	media := resp.Value.Content.Get("application/json")
	if media == nil || media.Schema == nil || !strings.HasSuffix(media.Schema.Ref, "#/components/schemas/"+errorResponseSchemaName) {
		return nil
	}
	return media.Schema
}

// AssertErrorResponses asserts that doc describes the error responses written by the openapi package:
//   - every operation taking parameters or a request body declares a 400 response with the error response schema,
//     because the request can fail validation before reaching the handler
//   - every sample, serialized as fxhttp.Error does on the wire, validates against the declared schemas
//
// Pass samples with the extensions your service uses, to assert the schema describes them
func AssertErrorResponses(tb testing.TB, doc *openapi3.T, samples ...*fxhttp.Error) {
	tb.Helper()

	if len(samples) == 0 {
		samples = []*fxhttp.Error{{Status: http.StatusBadRequest, Detail: "invalid request"}}
	}

	bodies := make([]any, 0, len(samples))
	for _, sample := range samples {
		data, err := json.Marshal(sample)
		require.NoError(tb, err)
		var body any
		require.NoError(tb, json.Unmarshal(data, &body))
		bodies = append(bodies, body)
	}

	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			schema := ErrorResponseSchema(op)
			if schema == nil {
				if len(item.Parameters) > 0 || len(op.Parameters) > 0 || op.RequestBody != nil {
					require.Failf(tb, "missing error response", "%s %s (%s) must declare a 400 response with the %s schema", method, path, op.OperationID, errorResponseSchemaName)
				}
				continue
			}
			for _, body := range bodies {
				require.NoError(tb, schema.Value.VisitJSON(body), "%s %s (%s): error response does not match the %s schema: %v", method, path, op.OperationID, errorResponseSchemaName, body)
			}
		}
	}
}
