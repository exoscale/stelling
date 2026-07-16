package api_test

import (
	"testing"

	"github.com/exoscale/stelling/examples/api"
	"github.com/exoscale/stelling/examples/api/internal"
	"github.com/exoscale/stelling/examples/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestHealthz_Success(t *testing.T) {
	server := api.NewAPIServer(
		&config.Config{},
		zaptest.NewLogger(t),
	)

	response, err := server.Healthz(t.Context(), internal.HealthzRequestObject{})
	require.NoError(t, err)

	resp200, ok := response.(internal.Healthz200JSONResponse)
	require.True(t, ok)
	assert.Equal(t, "OK", resp200.Message)
}
