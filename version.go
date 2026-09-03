package stelling

// Revision and RevisionTimestamp allow setting the vcs revision and timestamp at link time
// `go build -ldflags="-X 'github.com/exoscale/stelling.Revision=v1.0.0'"`
// `go build -ldflags="-X 'github.com/exoscale/stelling.RevisionTimestamp=2024-06-18T14:28:57Z'"`
// Other modules will pick up the revisions from here
// If not set, stelling modules will try to pick up the values from the debug BuildInfo
// This is useful if your build environment does not include vcs info (for caching or
// reproducibility reasons)
var Revision, RevisionTimestamp = "unknown", "unknown"
