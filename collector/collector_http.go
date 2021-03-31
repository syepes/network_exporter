package collector

import (
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
)

// HTTPGet prom
type HTTPGet struct {
	Monitor *monitor.HTTPGet
	metrics map[string]*http.HTTPReturn
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

	if m := p.Monitor.Export(); len(m) > 0 {
		p.metrics = m
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

		if metric.Success == true {
			ch <- prometheus.MustNewConstMetric(httpStatusDesc, prometheus.GaugeValue, float64(metric.Status), l...)
		} else {
			ch <- prometheus.MustNewConstMetric(httpStatusDesc, prometheus.GaugeValue, 0, l...)
		}

		ch <- prometheus.MustNewConstMetric(httpSizeDesc, prometheus.GaugeValue, float64(metric.ContentLength), l...)
		ch <- prometheus.MustNewConstMetric(httpTimeDesc, prometheus.GaugeValue, metric.DNSLookup.Seconds(), append(l, "DNSLookup")...)
		ch <- prometheus.MustNewConstMetric(httpTimeDesc, prometheus.GaugeValue, metric.TCPConnection.Seconds(), append(l, "TCPConnection")...)
		ch <- prometheus.MustNewConstMetric(httpTimeDesc, prometheus.GaugeValue, metric.TLSHandshake.Seconds(), append(l, "TLSHandshake")...)
		if !metric.TLSEarliestCertExpiry.IsZero() {
			ch <- prometheus.MustNewConstMetric(httpTimeDesc, prometheus.GaugeValue, float64(metric.TLSEarliestCertExpiry.Unix()), append(l, "TLSEarliestCertExpiry")...)
		}
		if !metric.TLSLastChainExpiry.IsZero() {
			ch <- prometheus.MustNewConstMetric(httpTimeDesc, prometheus.GaugeValue, float64(metric.TLSLastChainExpiry.Unix()), append(l, "TLSLastChainExpiry")...)
		}
		ch <- prometheus.MustNewConstMetric(httpTimeDesc, prometheus.GaugeValue, metric.ServerProcessing.Seconds(), append(l, "ServerProcessing")...)
		ch <- prometheus.MustNewConstMetric(httpTimeDesc, prometheus.GaugeValue, metric.ContentTransfer.Seconds(), append(l, "ContentTransfer")...)
		ch <- prometheus.MustNewConstMetric(httpTimeDesc, prometheus.GaugeValue, metric.Total.Seconds(), append(l, "Total")...)
	}
	ch <- prometheus.MustNewConstMetric(httpTargetsDesc, prometheus.GaugeValue, float64(len(targets)))
}
