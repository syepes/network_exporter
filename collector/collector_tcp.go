package collector

import (
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/syepes/network_exporter/pkg/common"
	"github.com/syepes/network_exporter/pkg/selfmon"
	"github.com/syepes/network_exporter/pkg/tcp"
)

var (
	tcpLabelNames  = []string{"name", "target", "target_ip", "source_ip", "port"}
	tcpTimeDesc    = prometheus.NewDesc("tcp_connection_seconds", "Connection time in seconds", tcpLabelNames, nil)
	tcpStatusDesc  = prometheus.NewDesc("tcp_connection_status", "Connection Status", tcpLabelNames, nil)
	tcpTargetsDesc = prometheus.NewDesc("tcp_targets", "Number of active targets", nil, nil)
	tcpStateDesc   = prometheus.NewDesc("tcp_up", "Exporter state", nil, nil)
	tcpMutex       = &sync.Mutex{}
	// Descriptor cache keyed by target identity (name); see collector_mtr.go for
	// the rationale behind identity keying plus a labelsEqual reload check.
	tcpDescCache      = make(map[string]*tcpDescCacheEntry)
	tcpDescCacheMutex sync.RWMutex
)

// tcpDescriptorSet holds all descriptors for a specific label set
type tcpDescriptorSet struct {
	time   *prometheus.Desc
	status *prometheus.Desc
}

// tcpDescCacheEntry stores a target's descriptors with the labels they were
// built from so a reload that changes labels forces a rebuild.
type tcpDescCacheEntry struct {
	labels prometheus.Labels
	descs  *tcpDescriptorSet
}

// getTCPDescriptors returns cached or creates new descriptors for a target.
func getTCPDescriptors(name string, labels prometheus.Labels) *tcpDescriptorSet {
	tcpDescCacheMutex.RLock()
	if e, ok := tcpDescCache[name]; ok && labelsEqual(e.labels, labels) {
		tcpDescCacheMutex.RUnlock()
		return e.descs
	}
	tcpDescCacheMutex.RUnlock()

	tcpDescCacheMutex.Lock()
	defer tcpDescCacheMutex.Unlock()

	if e, ok := tcpDescCache[name]; ok && labelsEqual(e.labels, labels) {
		return e.descs
	}

	descSet := &tcpDescriptorSet{
		time:   prometheus.NewDesc("tcp_connection_seconds", "Connection time in seconds", tcpLabelNames, labels),
		status: prometheus.NewDesc("tcp_connection_status", "Connection Status", tcpLabelNames, labels),
	}
	tcpDescCache[name] = &tcpDescCacheEntry{labels: labels, descs: descSet}
	return descSet
}

// evictTCPDescriptors drops descriptor-cache entries for inactive targets.
func evictTCPDescriptors(active map[string]struct{}) {
	tcpDescCacheMutex.Lock()
	defer tcpDescCacheMutex.Unlock()
	pruneDescCache(tcpDescCache, active)
}

// TCPMonitor is the subset of *monitor.TCPPort that the collector depends on.
type TCPMonitor interface {
	Snapshot() common.Snapshot[tcp.TCPPortReturn]
}

// TCP prom
type TCP struct {
	Monitor TCPMonitor
	metrics map[string]*tcp.TCPPortReturn
	labels  map[string]map[string]string
}

// Describe prom
func (p *TCP) Describe(ch chan<- *prometheus.Desc) {
	ch <- tcpTimeDesc
	ch <- tcpStatusDesc
	ch <- tcpTargetsDesc
	ch <- tcpStateDesc
}

// Collect prom
func (p *TCP) Collect(ch chan<- prometheus.Metric) {
	defer selfmon.ObserveScrape(selfmon.TypeTCP, time.Now())

	// Take one consistent snapshot of the live targets, then reconcile it against
	// the collector's cache so that targets removed at runtime (e.g. on a SIGHUP
	// config reload) stop being exported, while newly added targets appear once
	// they produce a result. reconcile returns fresh maps, so the lock only needs
	// to guard the pointer swap; the emission below runs lock-free over the local
	// maps, which no concurrent scrape mutates.
	snap := p.Monitor.Snapshot()
	active := newActiveSet(snap.Names)

	tcpMutex.Lock()
	metrics := reconcile(p.metrics, snap.Metrics, active)
	labels := reconcile(p.labels, snap.Labels, active)
	p.metrics = metrics
	p.labels = labels
	tcpMutex.Unlock()

	evictTCPDescriptors(active)

	if len(metrics) > 0 {
		ch <- prometheus.MustNewConstMetric(tcpStateDesc, prometheus.GaugeValue, 1)
	} else {
		ch <- prometheus.MustNewConstMetric(tcpStateDesc, prometheus.GaugeValue, 0)
	}

	for target, metric := range metrics {
		namePart := strings.SplitN(target, " ", 2)[0] // name without trailing ip

		descs := getTCPDescriptors(target, prometheus.Labels(labels[target]))

		// Both metrics share the full [name, target, target_ip, source_ip, port]
		// label set, built once per target.
		l := []string{namePart, metric.DestAddr, metric.DestIp, metric.SrcIp, metric.DestPort}

		ch <- prometheus.MustNewConstMetric(descs.time, prometheus.GaugeValue, metric.ConTime.Seconds(), l...)

		if metric.Success {
			ch <- prometheus.MustNewConstMetric(descs.status, prometheus.GaugeValue, 1, l...)
		} else {
			ch <- prometheus.MustNewConstMetric(descs.status, prometheus.GaugeValue, 0, l...)
		}
	}
	ch <- prometheus.MustNewConstMetric(tcpTargetsDesc, prometheus.GaugeValue, float64(len(metrics)))
}
