package fxhttp

import (
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
		body, err := json.Marshal(&Error{Status: http.StatusConflict, Type: "#/thing/exists", Title: "Thing exists", Detail: "thing 42"})
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"#/thing/exists","title":"Thing exists","status":409,"detail":"thing 42"}`, string(body))
	})

	t.Run("Should apply defaults based on the status", func(t *testing.T) {
		body, err := json.Marshal(&Error{Status: http.StatusNotFound})
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"#/http-problem-types/resource-not-found","title":"Not Found","status":404,"detail":""}`, string(body))
	})

	t.Run("Should default to a 500", func(t *testing.T) {
		body, err := json.Marshal(&Error{Detail: "boom"})
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"#/http-problem-types/internal-server-error","title":"Internal Server Error","status":500,"detail":"boom"}`, string(body))
	})

	t.Run("Should use about:blank for statuses without a default type", func(t *testing.T) {
		body, err := json.Marshal(&Error{Status: http.StatusTeapot})
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"about:blank","title":"I'm a teapot","status":418,"detail":""}`, string(body))
	})

	t.Run("Should serialize extensions as top level members", func(t *testing.T) {
		body, err := json.Marshal(&Error{
			Status:     http.StatusBadRequest,
			Extensions: map[string]any{"proxy-error": map[string]string{"error-name": "KeyNotFound"}},
		})
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"#/http-problem-types/request-invalid-parameters","title":"Bad Request","status":400,"detail":"","proxy-error":{"error-name":"KeyNotFound"}}`, string(body))
	})

	t.Run("Should not let extensions override standard members", func(t *testing.T) {
		body, err := json.Marshal(&Error{Status: http.StatusBadRequest, Detail: "real", Extensions: map[string]any{"status": 200, "detail": "fake"}})
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"#/http-problem-types/request-invalid-parameters","title":"Bad Request","status":400,"detail":"real"}`, string(body))
	})

	t.Run("Should never serialize the cause", func(t *testing.T) {
		body, err := json.Marshal(&Error{Status: http.StatusBadRequest, Cause: errors.New("secret")})
		require.NoError(t, err)
		require.NotContains(t, string(body), "secret")
	})
}

func TestErrorUnmarshalJSON(t *testing.T) {
	t.Run("Should round trip", func(t *testing.T) {
		in := &Error{Status: http.StatusConflict, Type: "#/thing/exists", Title: "Thing exists", Detail: "thing 42", Extensions: map[string]any{"thing-id": 42}}
		body, err := json.Marshal(in)
		require.NoError(t, err)

		out := &Error{}
		require.NoError(t, json.Unmarshal(body, out))
		require.Equal(t, in.Status, out.Status)
		require.Equal(t, in.Type, out.Type)
		require.Equal(t, in.Title, out.Title)
		require.Equal(t, in.Detail, out.Detail)
		require.Equal(t, json.RawMessage(`42`), out.Extensions["thing-id"])
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
		require.Equal(t, &Error{Status: http.StatusNotFound, Type: typeNotFound, Title: "Not Found", Detail: "no thing"}, e)
	})

	t.Run("Should not modify the original *Error", func(t *testing.T) {
		orig := &Error{Status: http.StatusNotFound}
		_ = errorFrom(orig)
		require.Equal(t, &Error{Status: http.StatusNotFound}, orig)
	})

	t.Run("Should map any other error to a 500 with its message", func(t *testing.T) {
		cause := errors.New("database unavailable")
		e := errorFrom(cause)
		require.Equal(t, &Error{Status: http.StatusInternalServerError, Type: TypeInternalServer, Title: "Internal Server Error", Detail: "database unavailable", Cause: cause}, e)
	})
}

func TestWriteError(t *testing.T) {
	t.Run("Should write the error as json with its status", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteError(rec, nil, &Error{Status: http.StatusBadRequest, Detail: "invalid id"})

		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
		require.JSONEq(t, `{"type":"#/http-problem-types/request-invalid-parameters","title":"Bad Request","status":400,"detail":"invalid id"}`, rec.Body.String())
	})

	t.Run("Should write a 500 for regular errors", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteError(rec, nil, errors.New("boom"))

		require.Equal(t, http.StatusInternalServerError, rec.Code)
		require.JSONEq(t, `{"type":"#/http-problem-types/internal-server-error","title":"Internal Server Error","status":500,"detail":"boom"}`, rec.Body.String())
	})

	t.Run("Should drop extensions that can not be serialized", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteError(rec, nil, &Error{Status: http.StatusBadRequest, Extensions: map[string]any{"fn": func() {}}})

		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.NotContains(t, rec.Body.String(), "fn")
		require.JSONEq(t, `{"type":"#/http-problem-types/request-invalid-parameters","title":"Bad Request","status":400,"detail":""}`, rec.Body.String())
	})
}

func TestErrorFromAPIError(t *testing.T) {
	notFound := apierror.New(apierror.Type{ID: "#/thing/not-found", Category: apierror.NotFound, Title: "Thing not found"}, "no details.")

	t.Run("Should convert API errors with the default status of their category", func(t *testing.T) {
		err := fmt.Errorf("lookup: %w", notFound.Type.New("thing 42").WithExtension("thing-id", 42))
		e := errorFrom(err)
		require.Equal(t, http.StatusNotFound, e.Status)
		require.Equal(t, "#/thing/not-found", e.Type)
		require.Equal(t, "Thing not found", e.Title)
		require.Equal(t, "thing 42", e.Detail)
		require.Equal(t, map[string]any{"thing-id": 42}, e.Extensions)
		require.ErrorIs(t, e, notFound, "the API error is the cause")
	})

	t.Run("Should use the given status", func(t *testing.T) {
		require.Equal(t, http.StatusBadRequest, ErrorFromAPIError(notFound, http.StatusBadRequest).Status)
	})

	t.Run("Should write API errors", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteError(rec, nil, notFound)
		require.Equal(t, http.StatusNotFound, rec.Code)
		require.JSONEq(t, `{"type":"#/thing/not-found","title":"Thing not found","status":404,"detail":"no details."}`, rec.Body.String())
	})

	t.Run("Should map categories to http statuses", func(t *testing.T) {
		require.Equal(t, http.StatusInternalServerError, DefaultStatus(apierror.Internal))
		require.Equal(t, http.StatusConflict, DefaultStatus(apierror.Conflict))
		require.Equal(t, http.StatusNotFound, DefaultStatus(apierror.NotFound))
		require.Equal(t, http.StatusBadRequest, DefaultStatus(apierror.InvalidArgument))
	})
}

func TestErrorMarshalJSONValue(t *testing.T) {
	body, err := json.Marshal(Error{Status: http.StatusConflict, Detail: "thing 42"})
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"#/http-problem-types/conflict","title":"Conflict","status":409,"detail":"thing 42"}`, string(body), "values are serialized in the wire format too")
}
