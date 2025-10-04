package collector

import (
	"fmt"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/syepes/network_exporter/monitor"
	"github.com/syepes/network_exporter/pkg/http"
)

var (
	httpLabelNames  = []string{"name", "target"}
	httpTimeDesc    = prometheus.NewDesc("http_get_seconds", "HTTP Get Drill Down time in seconds", append(httpLabelNames, "type"), nil)
	httpSizeDesc    = prometheus.NewDesc("http_get_content_bytes", "HTTP Get Content Size in bytes", httpLabelNames, nil)
	httpStatusDesc  = prometheus.NewDesc("http_get_status", "HTTP Get Status", httpLabelNames, nil)
	httpTargetsDesc = prometheus.NewDesc("http_get_targets", "Number of active targets", nil, nil)
	httpStateDesc   = prometheus.NewDesc("http_get_up", "Exporter state", nil, nil)
	httpMutex       = &sync.Mutex{}
	// Descriptor cache for custom labels
	httpDescCache      = make(map[string]*httpDescriptorSet)
	httpDescCacheMutex sync.RWMutex
)

// httpDescriptorSet holds all descriptors for a specific label set
type httpDescriptorSet struct {
	time   *prometheus.Desc
	size   *prometheus.Desc
	status *prometheus.Desc
}

// getHTTPDescriptors returns cached or creates new descriptors for a label set
func getHTTPDescriptors(labels prometheus.Labels) *httpDescriptorSet {
	cacheKey := fmt.Sprintf("%v", labels)

	httpDescCacheMutex.RLock()
	if descSet, exists := httpDescCache[cacheKey]; exists {
		httpDescCacheMutex.RUnlock()
		return descSet
	}
	httpDescCacheMutex.RUnlock()

	httpDescCacheMutex.Lock()
	defer httpDescCacheMutex.Unlock()

	if descSet, exists := httpDescCache[cacheKey]; exists {
		return descSet
	}

	descSet := &httpDescriptorSet{
		time:   prometheus.NewDesc("http_get_seconds", "HTTP Get Drill Down time in seconds", append(httpLabelNames, "type"), labels),
		size:   prometheus.NewDesc("http_get_content_bytes", "HTTP Get Content Size in bytes", httpLabelNames, labels),
		status: prometheus.NewDesc("http_get_status", "HTTP Get Status", httpLabelNames, labels),
	}
	httpDescCache[cacheKey] = descSet
	return descSet
}

// HTTPGet prom
type HTTPGet struct {
	Monitor *monitor.HTTPGet
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
	httpMutex.Lock()
	defer httpMutex.Unlock()

	if m := p.Monitor.ExportMetrics(); len(m) > 0 {
		p.metrics = m
	}

	if l := p.Monitor.ExportLabels(); len(l) > 0 {
		p.labels = l
	}

	if len(p.metrics) > 0 {
		ch <- prometheus.MustNewConstMetric(httpStateDesc, prometheus.GaugeValue, 1)
	} else {
		ch <- prometheus.MustNewConstMetric(httpStateDesc, prometheus.GaugeValue, 0)
	}

	targets := []string{}
	for target, metric := range p.metrics {
		targets = append(targets, target)
		l := strings.SplitN(target, " ", 2)
		l = append(l, metric.DestAddr)
		l2 := prometheus.Labels(p.labels[target])

		// Get cached descriptors for this label set
		descs := getHTTPDescriptors(l2)

		if metric.Success {
			ch <- prometheus.MustNewConstMetric(descs.status, prometheus.GaugeValue, float64(metric.Status), l...)
		} else {
			ch <- prometheus.MustNewConstMetric(descs.status, prometheus.GaugeValue, 0, l...)
		}

		ch <- prometheus.MustNewConstMetric(descs.size, prometheus.GaugeValue, float64(metric.ContentLength), l...)
		ch <- prometheus.MustNewConstMetric(descs.time, prometheus.GaugeValue, metric.DNSLookup.Seconds(), append(l, "DNSLookup")...)
		ch <- prometheus.MustNewConstMetric(descs.time, prometheus.GaugeValue, metric.TCPConnection.Seconds(), append(l, "TCPConnection")...)
		ch <- prometheus.MustNewConstMetric(descs.time, prometheus.GaugeValue, metric.TLSHandshake.Seconds(), append(l, "TLSHandshake")...)
		if !metric.TLSEarliestCertExpiry.IsZero() {
			ch <- prometheus.MustNewConstMetric(descs.time, prometheus.GaugeValue, float64(metric.TLSEarliestCertExpiry.Unix()), append(l, "TLSEarliestCertExpiry")...)
		}
		if !metric.TLSLastChainExpiry.IsZero() {
			ch <- prometheus.MustNewConstMetric(descs.time, prometheus.GaugeValue, float64(metric.TLSLastChainExpiry.Unix()), append(l, "TLSLastChainExpiry")...)
		}
		ch <- prometheus.MustNewConstMetric(descs.time, prometheus.GaugeValue, metric.ServerProcessing.Seconds(), append(l, "ServerProcessing")...)
		ch <- prometheus.MustNewConstMetric(descs.time, prometheus.GaugeValue, metric.ContentTransfer.Seconds(), append(l, "ContentTransfer")...)
		ch <- prometheus.MustNewConstMetric(descs.time, prometheus.GaugeValue, metric.Total.Seconds(), append(l, "Total")...)
	}
	ch <- prometheus.MustNewConstMetric(httpTargetsDesc, prometheus.GaugeValue, float64(len(targets)))
}
