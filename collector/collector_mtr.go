package collector

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/syepes/network_exporter/pkg/common"
	"github.com/syepes/network_exporter/pkg/mtr"
	"github.com/syepes/network_exporter/pkg/selfmon"
)

var (
	mtrLabelNames  = []string{"name", "target", "ttl", "path"}
	mtrDesc        = prometheus.NewDesc("mtr_rtt_seconds", "Round Trip Time in seconds", append(mtrLabelNames, "type"), nil)
	mtrSntDesc     = prometheus.NewDesc("mtr_rtt_snt_count", "Round Trip Send Package Total", append(mtrLabelNames, "type"), nil)
	mtrSntFailDesc = prometheus.NewDesc("mtr_rtt_snt_fail_count", "Round Trip Send Package Fail Total", append(mtrLabelNames, "type"), nil)
	mtrSntTimeDesc = prometheus.NewDesc("mtr_rtt_snt_seconds", "Round Trip Send Package Time Total", append(mtrLabelNames, "type"), nil)
	mtrHopsDesc    = prometheus.NewDesc("mtr_hops", "Number of route hops", []string{"name", "target"}, nil)
	mtrTargetsDesc = prometheus.NewDesc("mtr_targets", "Number of active targets", nil, nil)
	mtrStateDesc   = prometheus.NewDesc("mtr_up", "Exporter state", nil, nil)
	mtrMutex       = &sync.Mutex{}
	// Descriptor cache keyed by target identity (name). Descriptors are fixed for
	// a target's lifetime and only change if its custom labels change on reload,
	// which labelsEqual detects cheaply without the old per-scrape reflection key.
	mtrDescCache      = make(map[string]*mtrDescCacheEntry)
	mtrDescCacheMutex sync.RWMutex
)

// mtrDescriptorSet holds all descriptors for a specific label set
type mtrDescriptorSet struct {
	rtt     *prometheus.Desc
	hops    *prometheus.Desc
	snt     *prometheus.Desc
	sntFail *prometheus.Desc
	sntTime *prometheus.Desc
}

// mtrDescCacheEntry stores the descriptors for a target together with the custom
// labels they were built from, so a reload that changes labels forces a rebuild.
type mtrDescCacheEntry struct {
	labels prometheus.Labels
	descs  *mtrDescriptorSet
}

// getMTRDescriptors returns cached or creates new descriptors for a target.
func getMTRDescriptors(name string, labels prometheus.Labels) *mtrDescriptorSet {
	mtrDescCacheMutex.RLock()
	if e, ok := mtrDescCache[name]; ok && labelsEqual(e.labels, labels) {
		mtrDescCacheMutex.RUnlock()
		return e.descs
	}
	mtrDescCacheMutex.RUnlock()

	mtrDescCacheMutex.Lock()
	defer mtrDescCacheMutex.Unlock()

	if e, ok := mtrDescCache[name]; ok && labelsEqual(e.labels, labels) {
		return e.descs
	}

	descSet := &mtrDescriptorSet{
		rtt:     prometheus.NewDesc("mtr_rtt_seconds", "Round Trip Time in seconds", append(mtrLabelNames, "type"), labels),
		hops:    prometheus.NewDesc("mtr_hops", "Number of route hops", []string{"name", "target"}, labels),
		snt:     prometheus.NewDesc("mtr_rtt_snt_count", "Round Trip Send Package Total", mtrLabelNames, labels),
		sntFail: prometheus.NewDesc("mtr_rtt_snt_fail_count", "Round Trip Send Package Fail Total", mtrLabelNames, labels),
		sntTime: prometheus.NewDesc("mtr_rtt_snt_seconds", "Round Trip Send Package Time Total", mtrLabelNames, labels),
	}
	mtrDescCache[name] = &mtrDescCacheEntry{labels: labels, descs: descSet}
	return descSet
}

// evictMTRDescriptors drops descriptor-cache entries for inactive targets.
func evictMTRDescriptors(active map[string]struct{}) {
	mtrDescCacheMutex.Lock()
	defer mtrDescCacheMutex.Unlock()
	pruneDescCache(mtrDescCache, active)
}

// MtrMonitor is the subset of *monitor.MTR that the collector depends on.
type MtrMonitor interface {
	Snapshot() common.Snapshot[mtr.MtrResult]
}

// MTR prom
type MTR struct {
	Monitor MtrMonitor
	metrics map[string]*mtr.MtrResult
	labels  map[string]map[string]string
}

// Describe prom
func (p *MTR) Describe(ch chan<- *prometheus.Desc) {
	ch <- mtrDesc
	ch <- mtrHopsDesc
	ch <- mtrTargetsDesc
	ch <- mtrStateDesc
}

// Collect prom
func (p *MTR) Collect(ch chan<- prometheus.Metric) {
	defer selfmon.ObserveScrape(selfmon.TypeMTR, time.Now())

	// Take one consistent snapshot of the live targets, then reconcile it against
	// the collector's cache so that targets removed at runtime (e.g. on a SIGHUP
	// config reload) stop being exported, while newly added targets appear once
	// they produce a result. reconcile returns fresh maps, so the lock only needs
	// to guard the pointer swap; the O(N x hops) emission below runs lock-free
	// over the local maps, which no concurrent scrape mutates.
	snap := p.Monitor.Snapshot()
	active := newActiveSet(snap.Names)

	mtrMutex.Lock()
	metrics := reconcile(p.metrics, snap.Metrics, active)
	labels := reconcile(p.labels, snap.Labels, active)
	p.metrics = metrics
	p.labels = labels
	mtrMutex.Unlock()

	evictMTRDescriptors(active)

	if len(metrics) > 0 {
		ch <- prometheus.MustNewConstMetric(mtrStateDesc, prometheus.GaugeValue, 1)
	} else {
		ch <- prometheus.MustNewConstMetric(mtrStateDesc, prometheus.GaugeValue, 0)
	}

	for target, metric := range metrics {
		descs := getMTRDescriptors(target, prometheus.Labels(labels[target]))

		// hops metric uses the base [name, target] label set.
		ch <- prometheus.MustNewConstMetric(descs.hops, prometheus.GaugeValue, float64(len(metric.Hops)), target, metric.DestAddr)

		// rtt metrics share a single reused [name, target, ttl, path, type] slice;
		// only the varying slots are overwritten per emission. MustNewConstMetric
		// copies the label values synchronously, so reusing the backing array is
		// safe and avoids reallocating it for every metric.
		rttLabels := []string{target, metric.DestAddr, "", "", ""}
		for _, hop := range metric.Hops {
			rttLabels[2] = strconv.Itoa(hop.TTL)
			rttLabels[3] = hop.AddressTo
			rttLabels[4] = "last"
			ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, hop.LastTime.Seconds(), rttLabels...)
			rttLabels[4] = "sum"
			ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, hop.SumTime.Seconds(), rttLabels...)
			rttLabels[4] = "best"
			ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, hop.BestTime.Seconds(), rttLabels...)
			rttLabels[4] = "mean"
			ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, hop.AvgTime.Seconds(), rttLabels...)
			rttLabels[4] = "worst"
			ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, hop.WorstTime.Seconds(), rttLabels...)
			rttLabels[4] = "usd"
			ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, hop.UncorrectedSDTime.Seconds(), rttLabels...)
			rttLabels[4] = "csd"
			ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, hop.CorrectedSDTime.Seconds(), rttLabels...)
			rttLabels[4] = "range"
			ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, hop.RangeTime.Seconds(), rttLabels...)
			rttLabels[4] = "loss"
			ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, float64(hop.Loss), rttLabels...)
		}

		// snt* metrics share a reused [name, target, ttl, path] slice.
		sntLabels := []string{target, metric.DestAddr, "", ""}
		for ttl, summary := range metric.HopSummaryMap {
			ttlNum, _, _ := strings.Cut(ttl, "_")
			sntLabels[2] = ttlNum
			sntLabels[3] = summary.AddressTo
			ch <- prometheus.MustNewConstMetric(descs.snt, prometheus.CounterValue, float64(summary.Snt), sntLabels...)
			ch <- prometheus.MustNewConstMetric(descs.sntFail, prometheus.CounterValue, float64(summary.SntFail), sntLabels...)
			ch <- prometheus.MustNewConstMetric(descs.sntTime, prometheus.CounterValue, summary.SntTime.Seconds(), sntLabels...)
		}
	}
	ch <- prometheus.MustNewConstMetric(mtrTargetsDesc, prometheus.GaugeValue, float64(len(metrics)))
}
