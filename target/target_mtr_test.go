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
