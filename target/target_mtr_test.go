package target

import (
	"strconv"
	"sync"
	"testing"

	"github.com/syepes/network_exporter/pkg/common"
	"github.com/syepes/network_exporter/pkg/mtr"
)

// TestMTRComputeSnapshotIsolation reproduces the fatal MTR map race: the probe
// goroutine mutates the target's live HopSummaryMap in place while the collector
// iterates the result returned by Compute(). If Compute() returned the live
// pointer, this would trigger the unrecoverable "concurrent map read and map
// write" runtime error and crash the test binary. With the deep-copy snapshot
// the reader only ever touches its own copy, so the test completes cleanly
// (also verified under -race).
func TestMTRComputeSnapshotIsolation(t *testing.T) {
	tgt := &MTR{
		result: &mtr.MtrResult{
			DestAddr:      "10.0.0.9",
			Hops:          []common.IcmpHop{{TTL: 1, AddressTo: "10.0.0.1"}},
			HopSummaryMap: map[string]*common.IcmpSummary{},
		},
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writer: mimic mtr()'s in-place accumulation, guarded by the write lock.
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
			}
			tgt.Lock()
			key := strconv.Itoa(i%16) + "_10.0.0.1"
			s := tgt.result.HopSummaryMap[key]
			if s == nil {
				s = &common.IcmpSummary{AddressTo: "10.0.0.1"}
				tgt.result.HopSummaryMap[key] = s
			}
			s.Snt++
			s.SntFail++
			tgt.result.Hops = append(tgt.result.Hops[:1], common.IcmpHop{TTL: i % 16, AddressTo: "10.0.0.1"})
			tgt.Unlock()
			i++
		}
	}()

	// Reader: mimic the collector, which ranges the returned result with no
	// target lock held.
	for r := 0; r < 2000; r++ {
		snap := tgt.Compute()
		if snap == nil {
			continue
		}
		total := 0
		for _, s := range snap.HopSummaryMap {
			total += s.Snt + s.SntFail
		}
		for range snap.Hops {
			total++
		}
		_ = total
	}

	close(stop)
	wg.Wait()
}

// newMergeTarget builds a minimal MTR target suitable for exercising mergeHops
// directly (no goroutine, no network).
func newMergeTarget() *MTR {
	return &MTR{
		result:      &mtr.MtrResult{HopSummaryMap: map[string]*common.IcmpSummary{}},
		hopLastSeen: map[string]uint64{},
	}
}

func hopResult(dst string, hops ...common.IcmpHop) *mtr.MtrResult {
	return &mtr.MtrResult{DestAddr: dst, Hops: hops}
}

// TestMergeHopsAccumulatesMonotonically verifies that a hop seen every cycle has
// its cumulative counters summed rather than reset, so the exported counters stay
// monotonic.
func TestMergeHopsAccumulatesMonotonically(t *testing.T) {
	tgt := newMergeTarget()
	hop := common.IcmpHop{TTL: 1, AddressFrom: "10.0.0.254", AddressTo: "10.0.0.1", Snt: 5, SntFail: 1}

	for i := 0; i < 3; i++ {
		tgt.Lock()
		tgt.mergeHops(hopResult("10.0.0.9", hop))
		tgt.Unlock()
	}

	s := tgt.result.HopSummaryMap["1_10.0.0.1"]
	if s == nil {
		t.Fatalf("hop summary missing after repeated cycles")
	}
	if s.Snt != 15 {
		t.Fatalf("Snt = %d, want 15 (5 x 3 cycles)", s.Snt)
	}
	if s.SntFail != 3 {
		t.Fatalf("SntFail = %d, want 3", s.SntFail)
	}
}

// TestMergeHopsAgesOutStaleHops verifies that a hop which stops appearing (e.g.
// after a route change) is pruned from both the summary map and the last-seen
// bookkeeping once it has been absent for more than hopRetentionCycles, while a
// hop that keeps responding is retained.
func TestMergeHopsAgesOutStaleHops(t *testing.T) {
	tgt := newMergeTarget()
	stable := common.IcmpHop{TTL: 1, AddressTo: "10.0.0.1", Snt: 1}
	transient := common.IcmpHop{TTL: 2, AddressTo: "10.0.0.2", Snt: 1}

	// Cycle 1: both hops present.
	tgt.Lock()
	tgt.mergeHops(hopResult("10.0.0.9", stable, transient))
	tgt.Unlock()

	if _, ok := tgt.result.HopSummaryMap["2_10.0.0.2"]; !ok {
		t.Fatalf("transient hop should be present after first cycle")
	}

	// Subsequent cycles: only the stable hop responds. The transient hop must
	// survive until it has been absent for more than hopRetentionCycles, then be
	// pruned from both maps.
	for i := uint64(0); i < hopRetentionCycles+1; i++ {
		tgt.Lock()
		tgt.mergeHops(hopResult("10.0.0.9", stable))
		tgt.Unlock()
	}

	if _, ok := tgt.result.HopSummaryMap["1_10.0.0.1"]; !ok {
		t.Fatalf("stable hop was pruned but should be retained")
	}
	if _, ok := tgt.result.HopSummaryMap["2_10.0.0.2"]; ok {
		t.Fatalf("transient hop was not aged out of HopSummaryMap")
	}
	if _, ok := tgt.hopLastSeen["2_10.0.0.2"]; ok {
		t.Fatalf("transient hop was not aged out of hopLastSeen bookkeeping")
	}
}
