package main

import (
	"testing"

	"github.com/exoscale/stelling/examples/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestCreateSystem(t *testing.T) {
	// This test validates that the system contains all components
	// to create its dependency tree
	// If necessary multiple input configurations can be exercised

	conf := &config.Config{}
	t.Log(createSystem(conf))
	require.NoError(t, fx.ValidateApp(createSystem(conf)))
}
