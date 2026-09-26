package collector

// newActiveSet turns a slice of target names into a set for O(1) membership tests.
func newActiveSet(names []string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		set[n] = struct{}{}
	}
	return set
}

// reconcile merges the freshly exported values into the persistent cache and
// prunes every cached entry whose target is no longer active.
//
// This keeps the /metrics output in sync with the live target set when targets
// are added or removed at runtime (e.g. on a SIGHUP config reload):
//
//   - Targets present in fresh overwrite (or create) their cached value.
//   - Targets that are still active but produced no fresh value this scrape keep
//     their previously cached value (smooths transient gaps while a probe result
//     is pending).
//   - Targets absent from the active set are deleted, so removed targets stop
//     being exported immediately, even when all targets are removed at once.
//
// The (possibly newly allocated) cache is returned so callers can assign it back.
func reconcile[T any](cache map[string]T, fresh map[string]T, active map[string]struct{}) map[string]T {
	if cache == nil {
		cache = make(map[string]T, len(fresh))
	}

	for name, value := range fresh {
		cache[name] = value
	}

	for name := range cache {
		if _, ok := active[name]; !ok {
			delete(cache, name)
		}
	}

	return cache
}
