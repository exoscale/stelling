package fxmetrics

import (
	"runtime/debug"

	"github.com/prometheus/client_golang/prometheus"
)

// NewVersionCollector returns a collector collecting a single metric
// "go_version_info" with the constant value of 1 and 2 labels "revision"
// and "revision_timestamp". Their labels will contain the values of
// "vcs.revision" and "vcs.time" from the BuildInfo.Settings map or "unknown"
// if the vcs information is not available.
func NewVersionCollector() prometheus.Collector {
	revision, revisionTimestamp := "unknown", "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
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
