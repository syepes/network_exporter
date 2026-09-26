package common

// Snapshot is a consistent point-in-time view of a monitor's targets, captured
// under a single read lock so that a target cannot appear in one map but be
// missing from another. A sequence of separately-locked passes (the previous
// ExportMetrics / ExportLabels / TargetNames trio) could otherwise skew under a
// concurrent add/remove, and it re-locked every target several times per scrape.
type Snapshot[T any] struct {
	// Metrics holds the latest computed result for each target that has produced
	// one; targets still awaiting their first result are absent.
	Metrics map[string]*T
	// Labels holds the custom Prometheus labels for each target.
	Labels map[string]map[string]string
	// Names lists every live target, including those without a result yet, so the
	// collector can prune series for targets removed at runtime.
	Names []string
}
