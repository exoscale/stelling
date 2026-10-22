package apierror

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	ErrThingNotFound = New(Type{ID: "#/thing/not-found", Category: NotFound, Title: "Thing not found", Description: "The thing does not exist."}, "no details.")
	ErrThingLocked   = New(Type{ID: "#/thing/locked", Category: Conflict, Title: "Thing locked"}, "no details.")
)

func TestSentinels(t *testing.T) {
	t.Run("Should match occurrences of the same type", func(t *testing.T) {
		err := fmt.Errorf("lookup: %w", ErrThingNotFound.Type.New("thing 42"))
		require.ErrorIs(t, err, ErrThingNotFound)
		require.NotErrorIs(t, err, ErrThingLocked)
	})

	t.Run("Should match the sentinel itself", func(t *testing.T) {
		require.ErrorIs(t, ErrThingNotFound, ErrThingNotFound)
	})

	t.Run("Should keep the cause in the chain", func(t *testing.T) {
		cause := errors.New("connection refused")
		err := ErrThingNotFound.Type.Wrap("store unavailable", cause)
		require.ErrorIs(t, err, cause)
		require.Equal(t, "Thing not found: store unavailable: connection refused", err.Error())
	})
}

func TestAccessors(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", ErrThingLocked.Type.New("migration running"))

	require.Equal(t, "migration running", From(err).Detail)
	require.Equal(t, ErrThingLocked.Type, TypeOf(err))
	require.Equal(t, Conflict, CategoryOf(err))

	require.Nil(t, From(errors.New("plain")))
	require.Nil(t, TypeOf(errors.New("plain")))
	require.Equal(t, Internal, CategoryOf(errors.New("plain")), "errors which are not API errors are internal")
}

func TestWithExtension(t *testing.T) {
	err := ErrThingLocked.WithExtension("owner", "migration")
	require.Equal(t, map[string]any{"owner": "migration"}, err.Extensions)
	require.Nil(t, ErrThingLocked.Extensions, "the sentinel must not be modified")
	require.ErrorIs(t, err, ErrThingLocked)
}

func TestCategory(t *testing.T) {
	require.Equal(t, Internal, Category(0), "the zero value is internal")
	for _, c := range []Category{InvalidArgument, NotFound, Conflict} {
		require.True(t, c.IsClient())
	}
	require.False(t, Internal.IsClient())
}
