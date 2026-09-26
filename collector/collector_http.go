package collector

import (
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/syepes/network_exporter/pkg/common"
	"github.com/syepes/network_exporter/pkg/http"
	"github.com/syepes/network_exporter/pkg/selfmon"
)

var (
	httpLabelNames  = []string{"name", "target"}
	httpTimeDesc    = prometheus.NewDesc("http_get_seconds", "HTTP Get Drill Down time in seconds", append(httpLabelNames, "type"), nil)
	httpSizeDesc    = prometheus.NewDesc("http_get_content_bytes", "HTTP Get Content Size in bytes", httpLabelNames, nil)
	httpStatusDesc  = prometheus.NewDesc("http_get_status", "HTTP Get Status", httpLabelNames, nil)
	httpTargetsDesc = prometheus.NewDesc("http_get_targets", "Number of active targets", nil, nil)
	httpStateDesc   = prometheus.NewDesc("http_get_up", "Exporter state", nil, nil)
	httpMutex       = &sync.Mutex{}
	// Descriptor cache keyed by target identity (name); see collector_mtr.go for
	// the rationale behind identity keying plus a labelsEqual reload check.
	httpDescCache      = make(map[string]*httpDescCacheEntry)
	httpDescCacheMutex sync.RWMutex
)

// httpDescriptorSet holds all descriptors for a specific label set
type httpDescriptorSet struct {
	time   *prometheus.Desc
	size   *prometheus.Desc
	status *prometheus.Desc
}

// httpDescCacheEntry stores a target's descriptors with the labels they were
// built from so a reload that changes labels forces a rebuild.
type httpDescCacheEntry struct {
	labels prometheus.Labels
	descs  *httpDescriptorSet
}

// getHTTPDescriptors returns cached or creates new descriptors for a target.
func getHTTPDescriptors(name string, labels prometheus.Labels) *httpDescriptorSet {
	httpDescCacheMutex.RLock()
	if e, ok := httpDescCache[name]; ok && labelsEqual(e.labels, labels) {
		httpDescCacheMutex.RUnlock()
		return e.descs
	}
	httpDescCacheMutex.RUnlock()

	httpDescCacheMutex.Lock()
	defer httpDescCacheMutex.Unlock()

	if e, ok := httpDescCache[name]; ok && labelsEqual(e.labels, labels) {
		return e.descs
	}

	descSet := &httpDescriptorSet{
		time:   prometheus.NewDesc("http_get_seconds", "HTTP Get Drill Down time in seconds", append(httpLabelNames, "type"), labels),
		size:   prometheus.NewDesc("http_get_content_bytes", "HTTP Get Content Size in bytes", httpLabelNames, labels),
		status: prometheus.NewDesc("http_get_status", "HTTP Get Status", httpLabelNames, labels),
	}
	httpDescCache[name] = &httpDescCacheEntry{labels: labels, descs: descSet}
	return descSet
}

// evictHTTPDescriptors drops descriptor-cache entries for inactive targets.
func evictHTTPDescriptors(active map[string]struct{}) {
	httpDescCacheMutex.Lock()
	defer httpDescCacheMutex.Unlock()
	pruneDescCache(httpDescCache, active)
}

// HTTPMonitor is the subset of *monitor.HTTPGet that the collector depends on.
type HTTPMonitor interface {
	Snapshot() common.Snapshot[http.HTTPReturn]
}

// HTTPGet prom
type HTTPGet struct {
	Monitor HTTPMonitor
	metrics map[string]*http.HTTPReturn
	labels  map[string]map[string]string
}

// Describe prom
func (p *HTTPGet) Describe(ch chan<- *prometheus.Desc) {
	ch <- httpTimeDesc
	ch <- httpSizeDesc
	ch <- httpStatusDesc
	ch <- httpTargetsDesc
	ch <- httpStateDesc
}

// Collect prom
func (p *HTTPGet) Collect(ch chan<- prometheus.Metric) {
	defer selfmon.ObserveScrape(selfmon.TypeHTTP, time.Now())

	// Take one consistent snapshot of the live targets, then reconcile it against
	// the collector's cache so that targets removed at runtime (e.g. on a SIGHUP
	// config reload) stop being exported, while newly added targets appear once
	// they produce a result. reconcile returns fresh maps, so the lock only needs
	// to guard the pointer swap; the emission below runs lock-free over the local
	// maps, which no concurrent scrape mutates.
	snap := p.Monitor.Snapshot()
	active := newActiveSet(snap.Names)

	httpMutex.Lock()
	metrics := reconcile(p.metrics, snap.Metrics, active)
	labels := reconcile(p.labels, snap.Labels, active)
	p.metrics = metrics
	p.labels = labels
	httpMutex.Unlock()

	evictHTTPDescriptors(active)

	if len(metrics) > 0 {
		ch <- prometheus.MustNewConstMetric(httpStateDesc, prometheus.GaugeValue, 1)
	} else {
		ch <- prometheus.MustNewConstMetric(httpStateDesc, prometheus.GaugeValue, 0)
	}

	for target, metric := range metrics {
		descs := getHTTPDescriptors(target, prometheus.Labels(labels[target]))

		// Base [name, target] label set (status/size); the timing metrics reuse a
		// copy with one extra type slot that is overwritten per emission.
		l := strings.SplitN(target, " ", 2)
		l = append(l, metric.DestAddr)

		timeLabels := make([]string, len(l)+1)
		copy(timeLabels, l)
		typeIdx := len(l)

		if metric.Success {
			ch <- prometheus.MustNewConstMetric(descs.status, prometheus.GaugeValue, float64(metric.Status), l...)
		} else {
			ch <- prometheus.MustNewConstMetric(descs.status, prometheus.GaugeValue, 0, l...)
		}

		ch <- prometheus.MustNewConstMetric(descs.size, prometheus.GaugeValue, float64(metric.ContentLength), l...)
		timeLabels[typeIdx] = "DNSLookup"
		ch <- prometheus.MustNewConstMetric(descs.time, prometheus.GaugeValue, metric.DNSLookup.Seconds(), timeLabels...)
		timeLabels[typeIdx] = "TCPConnection"
		ch <- prometheus.MustNewConstMetric(descs.time, prometheus.GaugeValue, metric.TCPConnection.Seconds(), timeLabels...)
		timeLabels[typeIdx] = "TLSHandshake"
		ch <- prometheus.MustNewConstMetric(descs.time, prometheus.GaugeValue, metric.TLSHandshake.Seconds(), timeLabels...)
		if !metric.TLSEarliestCertExpiry.IsZero() {
			timeLabels[typeIdx] = "TLSEarliestCertExpiry"
			ch <- prometheus.MustNewConstMetric(descs.time, prometheus.GaugeValue, float64(metric.TLSEarliestCertExpiry.Unix()), timeLabels...)
		}
		if !metric.TLSLastChainExpiry.IsZero() {
			timeLabels[typeIdx] = "TLSLastChainExpiry"
			ch <- prometheus.MustNewConstMetric(descs.time, prometheus.GaugeValue, float64(metric.TLSLastChainExpiry.Unix()), timeLabels...)
		}
		timeLabels[typeIdx] = "ServerProcessing"
		ch <- prometheus.MustNewConstMetric(descs.time, prometheus.GaugeValue, metric.ServerProcessing.Seconds(), timeLabels...)
		timeLabels[typeIdx] = "ContentTransfer"
		ch <- prometheus.MustNewConstMetric(descs.time, prometheus.GaugeValue, metric.ContentTransfer.Seconds(), timeLabels...)
		timeLabels[typeIdx] = "Total"
		ch <- prometheus.MustNewConstMetric(descs.time, prometheus.GaugeValue, metric.Total.Seconds(), timeLabels...)
	}
	ch <- prometheus.MustNewConstMetric(httpTargetsDesc, prometheus.GaugeValue, float64(len(metrics)))
}
