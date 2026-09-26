package target

import (
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/syepes/network_exporter/pkg/common"
	"github.com/syepes/network_exporter/pkg/mtr"
	"github.com/syepes/network_exporter/pkg/selfmon"
)

// hopRetentionCycles bounds how long a hop entry survives in HopSummaryMap after
// it stops appearing in probe results. A route change or flap makes old
// (ttl, address) hops vanish from the current path; without aging they would
// accumulate forever, leaking heap, inflating the Compute() deep-copy, and
// leaking Prometheus series (3 x per stale hop). Hops that keep responding are
// refreshed every cycle and never pruned, so their cumulative counters stay
// monotonic; only hops absent for this many consecutive cycles are dropped.
const hopRetentionCycles uint64 = 100

// MTR Object
type MTR struct {
	logger            *slog.Logger
	icmpID            *common.IcmpID
	name              string
	host              string
	srcAddr           string
	interval          time.Duration
	timeout           time.Duration
	maxHops           int
	firstTTL          int
	count             int
	payloadSize       int
	protocol          string
	port              string
	ipv6              bool
	maxConcurrentJobs int
	labels            map[string]string
	result            *mtr.MtrResult
	// hopLastSeen records the probe cycle in which each HopSummaryMap key was last
	// observed, so stale hops can be aged out (see hopRetentionCycles). cycle is a
	// monotonic per-target probe-cycle counter, both guarded by the write lock.
	hopLastSeen map[string]uint64
	cycle       uint64
	stop        chan struct{}
	wg          sync.WaitGroup
	sync.RWMutex
}

// NewMTR starts a new monitoring goroutine
func NewMTR(logger *slog.Logger, icmpID *common.IcmpID, startupDelay time.Duration, name string, host string, srcAddr string, interval time.Duration, timeout time.Duration, maxHops int, firstTTL int, count int, payloadSize int, protocol string, port string, labels map[string]string, ipv6 bool, maxConcurrentJobs int) (*MTR, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	t := &MTR{
		logger:            logger,
		icmpID:            icmpID,
		name:              name,
		host:              host,
		srcAddr:           srcAddr,
		interval:          interval,
		timeout:           timeout,
		maxHops:           maxHops,
		firstTTL:          firstTTL,
		count:             count,
		payloadSize:       payloadSize,
		protocol:          protocol,
		port:              port,
		ipv6:              ipv6,
		maxConcurrentJobs: maxConcurrentJobs,
		labels:            labels,
		stop:              make(chan struct{}),
		result:            &mtr.MtrResult{HopSummaryMap: map[string]*common.IcmpSummary{}},
		hopLastSeen:       map[string]uint64{},
	}
	t.wg.Add(1)
	go t.run(startupDelay)
	return t, nil
}

func (t *MTR) run(startupDelay time.Duration) {
	if startupDelay > 0 {
		select {
		case <-time.After(startupDelay):
		case <-t.stop:
			t.wg.Done()
			return
		}
	}

	waitChan := make(chan struct{}, t.maxConcurrentJobs)

	// Execute first probe immediately (after jitter delay)
	// This ensures targets start probing as quickly as possible
	if !acquireSlot(selfmon.TypeMTR, waitChan, t.stop) {
		t.wg.Done()
		return
	}
	go func() {
		t.mtr()
		<-waitChan
	}()

	tick := time.NewTicker(t.interval)
	defer tick.Stop()

	for {
		select {
		case <-t.stop:
			t.wg.Done()
			return
		case <-tick.C:
			if !acquireSlot(selfmon.TypeMTR, waitChan, t.stop) {
				t.wg.Done()
				return
			}
			go func() {
				t.mtr()
				<-waitChan
			}()
		}
	}
}

// Stop gracefully stops the monitoring
func (t *MTR) Stop() {
	close(t.stop)
	t.wg.Wait()
}

func (t *MTR) mtr() {
	done := selfmon.ProbeStarted(selfmon.TypeMTR)
	icmpID := int(t.icmpID.Get())
	data, err := mtr.Mtr(t.host, t.srcAddr, t.maxHops, t.firstTTL, t.count, t.timeout, icmpID, t.payloadSize, t.protocol, t.port, t.ipv6)
	done(err == nil)
	if err != nil {
		t.logger.Error("MTR failed", "type", "MTR", "func", "mtr", "err", err)
	}

	t.Lock()
	defer t.Unlock()
	t.mergeHops(data)

	// Kept under the write lock: HopSummaryMap is accumulated in place across
	// cycles, so marshaling t.result after unlocking would race the next probe.
	// The Enabled guard inside logDebugResult skips the marshal entirely at the
	// default Info level, which is where the per-cycle cost mattered.
	logDebugResult(t.logger, "MTR result", "MTR", "mtr", t.result)
}

// mergeHops folds a fresh probe result into the target's cumulative
// HopSummaryMap and ages out hops that have stopped appearing. The caller must
// hold t's write lock. The summary map (Snt/SntFail/SntTime) is carried forward
// from the previous result so its counters remain monotonic across cycles; hops
// still present are refreshed, and any absent for hopRetentionCycles consecutive
// cycles are dropped so route flaps cannot leak heap or Prometheus series.
func (t *MTR) mergeHops(data *mtr.MtrResult) {
	summaryMap := t.result.HopSummaryMap
	if summaryMap == nil {
		summaryMap = map[string]*common.IcmpSummary{}
	}
	if t.hopLastSeen == nil {
		t.hopLastSeen = map[string]uint64{}
	}
	t.result = data
	t.cycle++
	for _, hop := range data.Hops {
		key := strconv.Itoa(hop.TTL) + "_" + hop.AddressTo
		summary := summaryMap[key]
		if summary == nil {
			summary = &common.IcmpSummary{}
			summaryMap[key] = summary
		}
		summary.AddressFrom = hop.AddressFrom
		summary.AddressTo = hop.AddressTo
		summary.Snt += hop.Snt
		summary.SntTime += hop.SumTime
		summary.SntFail += hop.SntFail
		t.hopLastSeen[key] = t.cycle
	}
	// seen <= cycle always holds, so the unsigned subtraction never underflows.
	for key, seen := range t.hopLastSeen {
		if t.cycle-seen > hopRetentionCycles {
			delete(summaryMap, key)
			delete(t.hopLastSeen, key)
		}
	}
	t.result.HopSummaryMap = summaryMap
}

// Compute returns an isolated deep copy of the MTR metrics.
//
// The target accumulates HopSummaryMap in place across probe cycles under the
// write lock, while the collector iterates the returned result without holding
// this target's lock. Returning the live pointer would let a scrape range the
// map at the same instant a probe writes it, which is an unrecoverable Go
// runtime "concurrent map read and map write" fatal error that crashes the
// whole exporter. Copying under RLock hands the collector an immutable snapshot
// while preserving the target-side lifetime accumulation. IcmpHop and
// IcmpSummary are all-value structs, so copying slice elements and map values
// by value is a full deep copy.
func (t *MTR) Compute() *mtr.MtrResult {
	t.RLock()
	defer t.RUnlock()

	if t.result == nil {
		return nil
	}

	snapshot := &mtr.MtrResult{DestAddr: t.result.DestAddr}
	if t.result.Hops != nil {
		snapshot.Hops = make([]common.IcmpHop, len(t.result.Hops))
		copy(snapshot.Hops, t.result.Hops)
	}
	if t.result.HopSummaryMap != nil {
		snapshot.HopSummaryMap = make(map[string]*common.IcmpSummary, len(t.result.HopSummaryMap))
		for k, v := range t.result.HopSummaryMap {
			if v == nil {
				snapshot.HopSummaryMap[k] = nil
				continue
			}
			summaryCopy := *v
			snapshot.HopSummaryMap[k] = &summaryCopy
		}
	}
	return snapshot
}

// Name returns name
func (t *MTR) Name() string {
	t.RLock()
	defer t.RUnlock()
	return t.name
}

// Host returns host
func (t *MTR) Host() string {
	t.RLock()
	defer t.RUnlock()
	return t.host
}

// Labels returns labels
func (t *MTR) Labels() map[string]string {
	t.RLock()
	defer t.RUnlock()
	return t.labels
}
