package fxhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/exoscale/stelling/fxhttp/apierror"
	"github.com/stretchr/testify/require"
)

func TestErrorMarshalJSON(t *testing.T) {
	t.Run("Should serialize the standard members", func(t *testing.T) {
		body, err := json.Marshal(&Error{Status: http.StatusConflict, Type: "#/problems/things/exists", Title: "The thing exists.", Detail: "thing 42", Instance: "req-1"})
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"#/problems/things/exists","title":"The thing exists.","status":409,"detail":"thing 42","instance":"req-1"}`, string(body))
	})

	t.Run("Should default to the shared type of the status, and omit empty members", func(t *testing.T) {
		body, err := json.Marshal(&Error{Status: http.StatusNotFound})
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"#/problems/not-found","title":"The resource is not found.","status":404}`, string(body))
	})

	t.Run("Should default to a 500", func(t *testing.T) {
		body, err := json.Marshal(&Error{Detail: "boom"})
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"#/problems/internal","title":"Internal Server Error","status":500,"detail":"boom"}`, string(body))
	})

	t.Run("Should give a shared type its title", func(t *testing.T) {
		body, err := json.Marshal(&Error{Status: http.StatusConflict, Type: apierror.Conflict.ID()})
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"#/problems/conflict","title":"The request conflicts with the current state.","status":409}`, string(body))
	})

	t.Run("Should use about:blank for statuses without a shared type", func(t *testing.T) {
		body, err := json.Marshal(&Error{Status: http.StatusTeapot})
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"about:blank","title":"I'm a teapot","status":418}`, string(body))
	})

	t.Run("Should serialize field errors", func(t *testing.T) {
		body, err := json.Marshal(&Error{Status: http.StatusBadRequest, Errors: []apierror.Field{
			apierror.F("coordinate", apierror.CodeRequired),
			apierror.FIn("query", "scope", apierror.CodeUnknownValue),
		}})
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"#/problems/invalid-request","title":"The request is not valid.","status":400,"errors":[
			{"name":"coordinate","code":"required","reason":"coordinate is required"},
			{"name":"scope","in":"query","code":"unknown-value","reason":"scope holds an unsupported value"}]}`, string(body))
	})

	t.Run("Should serialize extensions as top level members", func(t *testing.T) {
		body, err := json.Marshal(&Error{
			Status:     http.StatusBadRequest,
			Extensions: map[string]any{"proxy-error": map[string]string{"error-name": "KeyNotFound"}},
		})
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"#/problems/invalid-request","title":"The request is not valid.","status":400,"proxy-error":{"error-name":"KeyNotFound"}}`, string(body))
	})

	t.Run("Should not let extensions override standard members", func(t *testing.T) {
		body, err := json.Marshal(&Error{Status: http.StatusBadRequest, Detail: "real", Extensions: map[string]any{"status": 200, "detail": "fake"}})
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"#/problems/invalid-request","title":"The request is not valid.","status":400,"detail":"real"}`, string(body))
	})

	t.Run("Should never serialize the cause", func(t *testing.T) {
		body, err := json.Marshal(&Error{Status: http.StatusBadRequest, Cause: errors.New("secret")})
		require.NoError(t, err)
		require.NotContains(t, string(body), "secret")
	})

	t.Run("Should serialize values in the wire format too", func(t *testing.T) {
		body, err := json.Marshal(Error{Status: http.StatusConflict, Detail: "thing 42"})
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"#/problems/conflict","title":"The request conflicts with the current state.","status":409,"detail":"thing 42"}`, string(body))
	})
}

func TestErrorUnmarshalJSON(t *testing.T) {
	t.Run("Should round trip", func(t *testing.T) {
		in := &Error{Status: http.StatusConflict, Type: "#/problems/things/exists", Title: "The thing exists.", Detail: "thing 42", Instance: "req-1",
			Errors: []apierror.Field{apierror.F("name", apierror.CodeAlreadyExists)}, Extensions: map[string]any{"thing-id": 42}}
		body, err := json.Marshal(in)
		require.NoError(t, err)

		out := &Error{}
		require.NoError(t, json.Unmarshal(body, out))
		require.Equal(t, json.RawMessage(`42`), out.Extensions["thing-id"])
		out.Extensions = nil
		in.Extensions = nil
		require.Equal(t, in, out)
	})

	t.Run("Should keep errors in other shapes as an extension", func(t *testing.T) {
		for _, errors := range []string{`[{"detail":"must be read","location":"$.scope"}]`, `["bad"]`} {
			out := &Error{}
			require.NoError(t, json.Unmarshal([]byte(`{"type":"#/x","title":"X","status":400,"errors":`+errors+`}`), out))
			require.Equal(t, "#/x", out.Type)
			require.Empty(t, out.Errors)
			require.JSONEq(t, errors, string(out.Extensions["errors"].(json.RawMessage)))
		}
	})

	t.Run("Should fail on invalid standard members", func(t *testing.T) {
		require.Error(t, json.Unmarshal([]byte(`{"status":"400"}`), &Error{}))
	})
}

func TestErrorWrapping(t *testing.T) {
	cause := errors.New("cause")
	err := fmt.Errorf("wrapped: %w", &Error{Status: http.StatusBadRequest, Title: "Bad", Detail: "detail", Cause: cause})

	var e *Error
	require.ErrorAs(t, err, &e)
	require.ErrorIs(t, err, cause)
	require.Equal(t, "wrapped: Bad: detail: cause", err.Error())
}

func TestErrorFrom(t *testing.T) {
	t.Run("Should return a wrapped *Error with defaults applied", func(t *testing.T) {
		e := errorFrom(fmt.Errorf("wrapped: %w", &Error{Status: http.StatusNotFound, Detail: "no thing"}))
		require.Equal(t, &Error{Status: http.StatusNotFound, Type: apierror.NotFound.ID(), Title: apierror.NotFound.Title, Detail: "no thing"}, e)
	})

	t.Run("Should not modify the original *Error", func(t *testing.T) {
		orig := &Error{Status: http.StatusNotFound}
		_ = errorFrom(orig)
		require.Equal(t, &Error{Status: http.StatusNotFound}, orig)
	})

	t.Run("Should map any other error to a 500 without exposing its message", func(t *testing.T) {
		cause := errors.New("database unavailable")
		e := errorFrom(cause)
		require.Equal(t, &Error{Status: http.StatusInternalServerError, Type: apierror.Internal.ID(), Title: apierror.Internal.Title, Cause: cause}, e)
	})
}

func TestWriteError(t *testing.T) {
	t.Run("Should write the error as problem details with its status", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteError(rec, nil, &Error{Status: http.StatusBadRequest, Detail: "invalid id"})

		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
		require.Equal(t, "en", rec.Header().Get("Content-Language"))
		require.JSONEq(t, `{"type":"#/problems/invalid-request","title":"The request is not valid.","status":400,"detail":"invalid id"}`, rec.Body.String())
	})

	t.Run("Should set the request id as the instance", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.WithValue(context.Background(), requestIDCtxKey, "req-1"), http.MethodGet, "/", nil)
		WriteError(rec, req, &Error{Status: http.StatusNotFound})
		require.JSONEq(t, `{"type":"#/problems/not-found","title":"The resource is not found.","status":404,"instance":"req-1"}`, rec.Body.String())
	})

	t.Run("Should write a 500 without the message for regular errors", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteError(rec, nil, errors.New("boom"))

		require.Equal(t, http.StatusInternalServerError, rec.Code)
		require.JSONEq(t, `{"type":"#/problems/internal","title":"Internal Server Error","status":500}`, rec.Body.String())
	})

	t.Run("Should drop extensions that can not be serialized", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteError(rec, nil, &Error{Status: http.StatusBadRequest, Extensions: map[string]any{"fn": func() {}}})

		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.JSONEq(t, `{"type":"#/problems/invalid-request","title":"The request is not valid.","status":400}`, rec.Body.String())
	})
}

func TestErrorFromAPIError(t *testing.T) {
	notFound := apierror.New(apierror.Type{Slug: "things/not-found", Parent: apierror.NotFound, Title: "The thing is not found."}, "")

	t.Run("Should convert API errors with the status of their type", func(t *testing.T) {
		err := fmt.Errorf("lookup: %w", notFound.Type.New("thing 42").WithExtension("thing-id", 42))
		e := errorFrom(err)
		require.Equal(t, http.StatusNotFound, e.Status)
		require.Equal(t, "#/problems/things/not-found", e.Type)
		require.Equal(t, "The thing is not found.", e.Title)
		require.Equal(t, "thing 42", e.Detail)
		require.Equal(t, map[string]any{"thing-id": 42}, e.Extensions)
		require.ErrorIs(t, e, notFound, "the API error is the cause")
	})

	t.Run("Should write API errors with their fields", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteError(rec, nil, apierror.InvalidRequest.WithFields(apierror.F("coordinate", apierror.CodeRequired)))
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.JSONEq(t, `{"type":"#/problems/invalid-request","title":"The request is not valid.","status":400,
			"errors":[{"name":"coordinate","code":"required","reason":"coordinate is required"}]}`, rec.Body.String())
	})
}
