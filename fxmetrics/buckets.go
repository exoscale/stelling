package fxmetrics

// metricBuckets is the list of buckets used to categorize request latency
// The more there are the more precise the p50/p9x computation can be but the more metrics are exposed.
// We want stelling to be used in servers where requests can take between sub-ms time or several
// seconds per request.
var metricBuckets = []float64{
	0.000125, 0.00025, 0.0005,
	0.001, 0.002, 0.004, 0.008,
	0.016, 0.032, 0.064,
	0.128, 0.256, 0.512,
	1.024, 2.048, 4.096, 8.192}
