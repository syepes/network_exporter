package collector

import "github.com/prometheus/client_golang/prometheus"

// labelsEqual reports whether two custom-label sets are identical.
//
// The descriptor caches are keyed by target identity (name) rather than by a
// reflected fmt.Sprintf("%v", labels) string, which allocated and sorted on
// every scrape for every target. A target's custom labels only change across a
// config reload, so a cheap map comparison lets the cache detect that rare case
// and rebuild, while the common path is a single map lookup with no allocation.
func labelsEqual(a, b prometheus.Labels) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// pruneDescCache deletes descriptor-cache entries for targets no longer active,
// keeping the cache bounded by the live target set across reloads.
func pruneDescCache[T any](cache map[string]T, active map[string]struct{}) {
	for name := range cache {
		if _, ok := active[name]; !ok {
			delete(cache, name)
		}
	}
}

// newActiveSet turns a slice of target names into a set for O(1) membership tests.
func newActiveSet(names []string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		set[n] = struct{}{}
	}
	return set
}

// reconcile builds a fresh view of the active targets by overlaying the freshly
// exported values onto the previously cached ones, without mutating either input.
//
// This keeps the /metrics output in sync with the live target set when targets
// are added or removed at runtime (e.g. on a SIGHUP config reload):
//
//   - Targets present in fresh use (or create) their fresh value.
//   - Targets that are still active but produced no fresh value this scrape keep
//     their previously cached value (smooths transient gaps while a probe result
//     is pending).
//   - Targets absent from the active set are dropped, so removed targets stop
//     being exported immediately, even when all targets are removed at once.
//
// A brand-new map is returned every call so the caller can swap it in under a
// short lock and then range it lock-free during emission: because neither cache
// nor fresh is mutated, a previous scrape still ranging the old map cannot race
// this one building the next.
func reconcile[T any](cache map[string]T, fresh map[string]T, active map[string]struct{}) map[string]T {
	next := make(map[string]T, len(active))
	for name := range active {
		if value, ok := fresh[name]; ok {
			next[name] = value
		} else if value, ok := cache[name]; ok {
			next[name] = value
		}
	}
	return next
}
