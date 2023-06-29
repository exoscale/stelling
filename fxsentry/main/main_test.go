package main

import (
	"runtime/debug"
	"testing"

	_ "github.com/exoscale/stelling/fxsentry"
	"github.com/stretchr/testify/require"
)

// Our Sentry version must be 0.20.0
// Later versions of the SDK have dropped support for our hosted version
// (We run a very old version of sentry)
// Unfortunately this doesn't raise any errors on the client side, and
// so we will lose sentries without noticing
// Given that we regularly use `go get -u ./...` to ensure we run up to
// date dependencies, accidentally upgrading sentry is a real risk
// This test will inspect the BuildInfo and error out if sentry is not
// at the version we expect: this way we can test an accidental upgrade
// at test time, both locally and in CI
func TestSentryVersion(t *testing.T) {
	info, ok := debug.ReadBuildInfo()
	require.True(t, ok, "Must be able to read the build info")

	found := false
	t.Log(info.Deps)
	t.Log(info.Main)
	for _, mod := range info.Deps {
		if mod.Path == "github.com/getsentry/sentry-go" {
			found = true
			require.Equal(t, "v0.20.0", mod.Version, "You accidentally upgraded the github.com/getsentry/sentry-go dependency. It must be v0.20.0")
		}
	}
	require.True(t, found, "Package github.com/getsentry/sentry-go not found")
}
