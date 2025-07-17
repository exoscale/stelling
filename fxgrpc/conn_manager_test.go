package fxgrpc

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
)

func TestNewConnManagerModule(t *testing.T) {
	conf := &ConnManagerOpts{InsecureConnection: true}
	app := fxtest.New(
		t,
		NewConnManagerModule(conf),
		fx.Provide(zap.NewNop),
		fx.Invoke(func(m *ConnManager) {
			require.NotNil(t, m)
		}),
	)
	defer app.RequireStart().RequireStop()
}

// TODO: implement a test that tries to concurrently get connections
// We can spawn a small server on localhost to target
