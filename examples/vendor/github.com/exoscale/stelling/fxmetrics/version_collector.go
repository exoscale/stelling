package fxmetrics

import (
	"runtime/debug"

	"github.com/exoscale/stelling"
	"github.com/prometheus/client_golang/prometheus"
)

var revision, revisionTimestamp = stelling.Revision, stelling.RevisionTimestamp

// NewVersionCollector returns a collector collecting a single metric "go_version_info" with the
// constant value of 1 and 2 labels "revision" and "revision_timestamp".
// Their values can be set at link time via the Revision and RevisionTimestamp in the top level stelling package
// `go build -ldflags="-X 'github.com/exoscale/stelling.Revision=v1.0.0'"`
// `go build -ldflags="-X 'github.com/exoscale/stelling.RevisionTimestamp=2024-06-18T14:28:57Z'"`
// If not set at link time, the labels will contain the values of "vcs.revision" and "vcs.time" from
// the BuildInfo.Settings map.
// If neither way returns an output, the value will be set to "unknown".
func NewVersionCollector() prometheus.GaugeFunc {
	if info, ok := debug.ReadBuildInfo(); ok && revision == "unknown" {
		for _, item := range info.Settings {
			switch item.Key {
			case "vcs.revision":
				revision = item.Value
			case "vcs.time":
				revisionTimestamp = item.Value
			}
		}
	}

	return prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "go_version_info",
			Help: "Version information about the main Go module.",
			ConstLabels: prometheus.Labels{
				"revision":           revision,
				"revision_timestamp": revisionTimestamp,
			},
		},
		func() float64 { return 1 },
	)
}
