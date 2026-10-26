package apierror

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	ErrThingNotFound = New(Type{Slug: "things/not-found", Parent: NotFound, Title: "The thing is not found.", Description: "The thing does not exist."}, "")
	ErrThingLocked   = New(Type{Slug: "things/locked", Parent: Conflict, Title: "The thing is locked."}, "")
)

func TestTypes(t *testing.T) {
	require.Equal(t, "#/problems/things/not-found", ErrThingNotFound.Type.ID())
	require.Equal(t, 404, ErrThingNotFound.Type.Status(), "service types take the status of their parent")
	require.Equal(t, 400, InvalidRequest.Status())
	require.True(t, ErrThingLocked.Type.IsClient())
	require.False(t, Internal.IsClient())
}

func TestValidate(t *testing.T) {
	require.NoError(t, Validate(ErrThingNotFound.Type, ErrThingLocked.Type))

	for name, tc := range map[string]struct {
		types []*Type
		err   string
	}{
		"slug without namespace": {[]*Type{{Slug: "locked", Parent: Conflict, Title: "Locked"}}, "<service>/<name>"},
		"slug not in kebab-case": {[]*Type{{Slug: "things/Locked", Parent: Conflict, Title: "Locked"}}, "<service>/<name>"},
		"several namespaces":     {[]*Type{ErrThingLocked.Type, {Slug: "other/locked", Parent: Conflict, Title: "Locked"}}, `under the namespace "things"`},
		"declared twice":         {[]*Type{ErrThingLocked.Type, ErrThingLocked.Type}, "declared twice"},
		"no parent":              {[]*Type{{Slug: "things/locked", Title: "Locked"}}, "parent must be a shared type"},
		"service type as parent": {[]*Type{{Slug: "things/stuck", Parent: ErrThingLocked.Type, Title: "Stuck"}}, "parent must be a shared type"},
		"no title":               {[]*Type{{Slug: "things/locked", Parent: Conflict}}, "requires a title"},
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorContains(t, Validate(tc.types...), tc.err)
		})
	}
}

func TestSentinels(t *testing.T) {
	t.Run("Should match occurrences of the same type", func(t *testing.T) {
		err := fmt.Errorf("lookup: %w", ErrThingNotFound.Type.New("thing 42"))
		require.ErrorIs(t, err, ErrThingNotFound)
		require.NotErrorIs(t, err, ErrThingLocked)
	})

	t.Run("Should keep the cause in the chain", func(t *testing.T) {
		cause := errors.New("connection refused")
		err := ErrThingNotFound.Type.Wrap("store unavailable", cause)
		require.ErrorIs(t, err, cause)
		require.Equal(t, "The thing is not found.: store unavailable: connection refused", err.Error())
	})
}

func TestFields(t *testing.T) {
	err := InvalidRequest.WithFields(F("rules[0].action", CodeNotAllowed), FIn("query", "scope", CodeUnknownValue))
	require.Equal(t, []Field{
		{Name: "rules[0].action", Code: CodeNotAllowed, Reason: "rules[0].action is not allowed here"},
		{Name: "scope", In: "query", Code: CodeUnknownValue, Reason: "scope holds an unsupported value"},
	}, err.Fields)
	require.Equal(t, "the request is not valid", CodeInvalidData.Reason(""), "form level errors have no name")
	require.True(t, CodeRequired.Known())
	require.False(t, Code("bad").Known())
}

func TestAccessors(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", ErrThingLocked.Type.New("migration running"))

	require.Equal(t, "migration running", From(err).Detail)
	require.Equal(t, ErrThingLocked.Type, TypeOf(err))

	require.Nil(t, From(errors.New("plain")))
	require.Equal(t, Internal, TypeOf(errors.New("plain")), "errors which are not API errors are internal")
}

func TestWithExtension(t *testing.T) {
	err := ErrThingLocked.WithExtension("owner", "migration")
	require.Equal(t, map[string]any{"owner": "migration"}, err.Extensions)
	require.Nil(t, ErrThingLocked.Extensions, "the sentinel must not be modified")
	require.ErrorIs(t, err, ErrThingLocked)
}
