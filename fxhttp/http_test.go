package fxhttp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRPCMethodFromContext(t *testing.T) {
	t.Run("Should return the empty string if no method is set", func(t *testing.T) {
		ctx := context.Background()
		require.Equal(t, "", RPCMethodFromContext(ctx))
	})

	t.Run("Should return the correct rpc method", func(t *testing.T) {
		expected := "exoscale.kms/ListKeys"
		ctx := injectRPCMethod(context.Background(), expected)
		require.Equal(t, expected, RPCMethodFromContext(ctx))
	})
}
