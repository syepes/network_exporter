package mtr

import (
	"testing"
	"time"
)

// ret is a small helper to build a probed *MtrReturn for a given TTL/host.
func ret(ttl int, host string, count, succSum int) *MtrReturn {
	return &MtrReturn{
		ttl:      ttl,
		host:     host,
		success:  true,
		succSum:  succSum,
		lastTime: time.Millisecond,
		allTime:  []time.Duration{time.Millisecond},
		sumTime:  time.Millisecond,
	}
}

// TestAggregateHops covers the TTL-indexed folding of probe results, focusing on
// the first-ttl offset introduced for the Cilium-friendly initial-TTL feature.
// mtrReturns is indexed by TTL: index 0 is synthetic and indices below firstTTL
// are never probed (nil).
func TestAggregateHops(t *testing.T) {
	const count = 6

	t.Run("firstTTL 1 regression, full path terminated by nil tail", func(t *testing.T) {
		mtrReturns := []*MtrReturn{
			nil,                          // 0 synthetic
			ret(1, "10.0.0.1", count, 6), // 1
			ret(2, "10.0.0.2", count, 5), // 2
			ret(3, "1.1.1.1", count, 4),  // 3
			nil,                          // 4 (un-probed tail)
		}
		hops := aggregateHops(mtrReturns, 1, count, "9.9.9.9")
		if len(hops) != 3 {
			t.Fatalf("expected 3 hops, got %d", len(hops))
		}
		// First hop references its own host (no predecessor).
		if hops[0].TTL != 1 || hops[0].AddressFrom != "10.0.0.1" || hops[0].AddressTo != "10.0.0.1" {
			t.Fatalf("hop[0] wrong: %+v", hops[0])
		}
		// Subsequent hops reference the previous host.
		if hops[1].AddressFrom != "10.0.0.1" || hops[1].AddressTo != "10.0.0.2" {
			t.Fatalf("hop[1] wrong: %+v", hops[1])
		}
		if hops[2].AddressFrom != "10.0.0.2" || hops[2].AddressTo != "1.1.1.1" {
			t.Fatalf("hop[2] wrong: %+v", hops[2])
		}
		// Snt / SntFail / Loss propagate the count.
		if hops[2].Snt != count || hops[2].SntFail != count-4 {
			t.Fatalf("hop[2] snt accounting wrong: %+v", hops[2])
		}
	})

	t.Run("firstTTL greater than 1 skips nil low indices without panic", func(t *testing.T) {
		mtrReturns := []*MtrReturn{
			nil,                         // 0
			nil,                         // 1 (un-probed, below firstTTL)
			nil,                         // 2 (un-probed, below firstTTL)
			ret(3, "a.a.a.a", count, 6), // 3 == firstTTL
			ret(4, "b.b.b.b", count, 6), // 4
			ret(5, "c.c.c.c", count, 6), // 5
			nil,                         // 6 tail
		}
		hops := aggregateHops(mtrReturns, 3, count, "9.9.9.9")
		if len(hops) != 3 {
			t.Fatalf("expected 3 hops, got %d", len(hops))
		}
		// The first reported hop is TTL 3 and references its own host (index 2 is
		// nil and must not be dereferenced).
		if hops[0].TTL != 3 || hops[0].AddressFrom != "a.a.a.a" {
			t.Fatalf("hop[0] wrong: %+v", hops[0])
		}
		if hops[1].AddressFrom != "a.a.a.a" || hops[2].AddressFrom != "b.b.b.b" {
			t.Fatalf("predecessor linkage wrong: %+v %+v", hops[1], hops[2])
		}
	})

	t.Run("stops when destination is reached mid-path", func(t *testing.T) {
		mtrReturns := []*MtrReturn{
			nil,
			ret(1, "10.0.0.1", count, 6),
			ret(2, "1.1.1.1", count, 6), // destination
			ret(3, "10.0.0.3", count, 6),
		}
		hops := aggregateHops(mtrReturns, 1, count, "1.1.1.1")
		if len(hops) != 2 {
			t.Fatalf("expected walk to stop at destination (2 hops), got %d", len(hops))
		}
		if hops[len(hops)-1].AddressTo != "1.1.1.1" {
			t.Fatalf("last hop should be the destination: %+v", hops[len(hops)-1])
		}
	})

	t.Run("empty when no indices at or above firstTTL are probed", func(t *testing.T) {
		mtrReturns := []*MtrReturn{nil, nil, nil}
		hops := aggregateHops(mtrReturns, 2, count, "9.9.9.9")
		if len(hops) != 0 {
			t.Fatalf("expected 0 hops, got %d", len(hops))
		}
	})

	// With the max-hops off-by-one fixed, the probe loop now fills the final
	// mtrReturns slot (index == MaxHops), so aggregateHops must fold a fully
	// probed array (no nil tail) without dropping the last hop or panicking.
	t.Run("max-hops boundary hop is aggregated when the last slot is probed", func(t *testing.T) {
		const maxHops = 4
		// mtrReturns is sized MaxHops+1 (indices 0..MaxHops); every TTL from
		// firstTTL..MaxHops is probed, so there is no nil tail to break on.
		mtrReturns := []*MtrReturn{
			nil,                          // 0 synthetic
			ret(1, "10.0.0.1", count, 6), // 1
			ret(2, "10.0.0.2", count, 6), // 2
			ret(3, "10.0.0.3", count, 6), // 3
			ret(4, "10.0.0.4", count, 6), // 4 == MaxHops
		}
		hops := aggregateHops(mtrReturns, 1, count, "9.9.9.9")
		if len(hops) != maxHops {
			t.Fatalf("expected %d hops (incl. the max-hops slot), got %d", maxHops, len(hops))
		}
		last := hops[len(hops)-1]
		if last.TTL != maxHops || last.AddressTo != "10.0.0.4" {
			t.Fatalf("expected last hop at TTL %d to 10.0.0.4, got %+v", maxHops, last)
		}
		if last.AddressFrom != "10.0.0.3" {
			t.Fatalf("expected last hop predecessor 10.0.0.3, got %+v", last)
		}
	})
}
