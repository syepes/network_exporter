package collector

import (
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/syepes/network_exporter/pkg/common"
	"github.com/syepes/network_exporter/pkg/ping"
	"github.com/syepes/network_exporter/pkg/selfmon"
)

var (
	icmpLabelNames         = []string{"name", "target", "target_ip"}
	icmpStatusDesc         = prometheus.NewDesc("ping_status", "Ping Status", icmpLabelNames, nil)
	icmpRttDesc            = prometheus.NewDesc("ping_rtt_seconds", "Round Trip Time in seconds", append(icmpLabelNames, "type"), nil)
	icmpSntSummaryDesc     = prometheus.NewDesc("ping_rtt_snt_count", "Packet sent count", icmpLabelNames, nil)
	icmpSntFailSummaryDesc = prometheus.NewDesc("ping_rtt_snt_fail_count", "Packet sent fail count", icmpLabelNames, nil)
	icmpSntTimeSummaryDesc = prometheus.NewDesc("ping_rtt_snt_seconds", "Packet sent time total", icmpLabelNames, nil)
	icmpLossDesc           = prometheus.NewDesc("ping_loss_percent", "Packet loss in percent", icmpLabelNames, nil)
	icmpTargetsDesc        = prometheus.NewDesc("ping_targets", "Number of active targets", nil, nil)
	icmpStateDesc          = prometheus.NewDesc("ping_up", "Exporter state", nil, nil)
	icmpMutex              = &sync.Mutex{}
	// Descriptor cache keyed by target identity (name); see collector_mtr.go for
	// the rationale behind identity keying plus a labelsEqual reload check.
	icmpDescCache      = make(map[string]*icmpDescCacheEntry)
	icmpDescCacheMutex sync.RWMutex
)

// descriptorSet holds all descriptors for a specific label set
type descriptorSet struct {
	status         *prometheus.Desc
	rtt            *prometheus.Desc
	sntSummary     *prometheus.Desc
	sntFailSummary *prometheus.Desc
	sntTimeSummary *prometheus.Desc
	loss           *prometheus.Desc
}

// icmpDescCacheEntry stores a target's descriptors with the labels they were
// built from so a reload that changes labels forces a rebuild.
type icmpDescCacheEntry struct {
	labels prometheus.Labels
	descs  *descriptorSet
}

// getDescriptors returns cached or creates new descriptors for a target.
func getDescriptors(name string, labels prometheus.Labels) *descriptorSet {
	icmpDescCacheMutex.RLock()
	if e, ok := icmpDescCache[name]; ok && labelsEqual(e.labels, labels) {
		icmpDescCacheMutex.RUnlock()
		return e.descs
	}
	icmpDescCacheMutex.RUnlock()

	icmpDescCacheMutex.Lock()
	defer icmpDescCacheMutex.Unlock()

	if e, ok := icmpDescCache[name]; ok && labelsEqual(e.labels, labels) {
		return e.descs
	}

	descSet := &descriptorSet{
		status:         prometheus.NewDesc("ping_status", "Ping Status", icmpLabelNames, labels),
		rtt:            prometheus.NewDesc("ping_rtt_seconds", "Round Trip Time in seconds", append(icmpLabelNames, "type"), labels),
		sntSummary:     prometheus.NewDesc("ping_rtt_snt_count", "Packet sent count", icmpLabelNames, labels),
		sntFailSummary: prometheus.NewDesc("ping_rtt_snt_fail_count", "Packet sent fail count", icmpLabelNames, labels),
		sntTimeSummary: prometheus.NewDesc("ping_rtt_snt_seconds", "Packet sent time total", icmpLabelNames, labels),
		loss:           prometheus.NewDesc("ping_loss_percent", "Packet loss in percent", icmpLabelNames, labels),
	}
	icmpDescCache[name] = &icmpDescCacheEntry{labels: labels, descs: descSet}
	return descSet
}

// evictPingDescriptors drops descriptor-cache entries for inactive targets.
func evictPingDescriptors(active map[string]struct{}) {
	icmpDescCacheMutex.Lock()
	defer icmpDescCacheMutex.Unlock()
	pruneDescCache(icmpDescCache, active)
}

// PingMonitor is the subset of *monitor.PING that the collector depends on.
type PingMonitor interface {
	Snapshot() common.Snapshot[ping.PingResult]
}

// PING prom
type PING struct {
	Monitor PingMonitor
	metrics map[string]*ping.PingResult
	labels  map[string]map[string]string
}

// Describe prom
func (p *PING) Describe(ch chan<- *prometheus.Desc) {
	ch <- icmpStatusDesc
	ch <- icmpRttDesc
	ch <- icmpLossDesc
	ch <- icmpTargetsDesc
	ch <- icmpStateDesc
}

// Collect prom
func (p *PING) Collect(ch chan<- prometheus.Metric) {
	defer selfmon.ObserveScrape(selfmon.TypePing, time.Now())

	// Take one consistent snapshot of the live targets, then reconcile it against
	// the collector's cache so that targets removed at runtime (e.g. on a SIGHUP
	// config reload) stop being exported, while newly added targets appear once
	// they produce a result. reconcile returns fresh maps, so the lock only needs
	// to guard the pointer swap; the emission below runs lock-free over the local
	// maps, which no concurrent scrape mutates.
	snap := p.Monitor.Snapshot()
	active := newActiveSet(snap.Names)

	icmpMutex.Lock()
	metrics := reconcile(p.metrics, snap.Metrics, active)
	labels := reconcile(p.labels, snap.Labels, active)
	p.metrics = metrics
	p.labels = labels
	icmpMutex.Unlock()

	evictPingDescriptors(active)

	if len(metrics) > 0 {
		ch <- prometheus.MustNewConstMetric(icmpStateDesc, prometheus.GaugeValue, 1)
	} else {
		ch <- prometheus.MustNewConstMetric(icmpStateDesc, prometheus.GaugeValue, 0)
	}

	for target, metric := range metrics {
		namePart := strings.SplitN(target, " ", 2)[0] // name without trailing ip

		descs := getDescriptors(target, prometheus.Labels(labels[target]))

		// Base [name, target, target_ip] label set reused by the non-typed metrics.
		l := []string{namePart, metric.DestAddr, metric.DestIp}

		if metric.Success {
			ch <- prometheus.MustNewConstMetric(descs.status, prometheus.GaugeValue, 1, l...)
		} else {
			ch <- prometheus.MustNewConstMetric(descs.status, prometheus.GaugeValue, 0, l...)
		}

		// rtt metrics reuse a single [name, target, target_ip, type] slice; only
		// the type slot is overwritten per emission (MustNewConstMetric copies the
		// values synchronously, so reusing the backing array is safe).
		rttLabels := []string{namePart, metric.DestAddr, metric.DestIp, ""}
		rttLabels[3] = "best"
		ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, metric.BestTime.Seconds(), rttLabels...)
		rttLabels[3] = "mean"
		ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, metric.AvgTime.Seconds(), rttLabels...)
		rttLabels[3] = "worst"
		ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, metric.WorstTime.Seconds(), rttLabels...)
		rttLabels[3] = "sum"
		ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, metric.SumTime.Seconds(), rttLabels...)
		rttLabels[3] = "usd"
		ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, metric.UncorrectedSDTime.Seconds(), rttLabels...)
		rttLabels[3] = "csd"
		ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, metric.CorrectedSDTime.Seconds(), rttLabels...)
		rttLabels[3] = "range"
		ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, metric.RangeTime.Seconds(), rttLabels...)

		ch <- prometheus.MustNewConstMetric(descs.sntSummary, prometheus.GaugeValue, float64(metric.SntSummary), l...)
		ch <- prometheus.MustNewConstMetric(descs.sntFailSummary, prometheus.GaugeValue, float64(metric.SntFailSummary), l...)
		ch <- prometheus.MustNewConstMetric(descs.sntTimeSummary, prometheus.GaugeValue, metric.SntTimeSummary.Seconds(), l...)
		ch <- prometheus.MustNewConstMetric(descs.loss, prometheus.GaugeValue, metric.DropRate, l...)
	}
	ch <- prometheus.MustNewConstMetric(icmpTargetsDesc, prometheus.GaugeValue, float64(len(metrics)))
}
