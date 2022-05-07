package collector

import (
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/syepes/network_exporter/monitor"
	"github.com/syepes/network_exporter/pkg/tcp"
)

var (
	tcpLabelNames  = []string{"name", "target", "target_ip", "port"}
	tcpTimeDesc    = prometheus.NewDesc("tcp_connection_seconds", "Connection time in seconds", tcpLabelNames, nil)
	tcpStatusDesc  = prometheus.NewDesc("tcp_connection_status", "Connection Status", tcpLabelNames, nil)
	tcpTargetsDesc = prometheus.NewDesc("tcp_targets", "Number of active targets", nil, nil)
	tcpStateDesc   = prometheus.NewDesc("tcp_up", "Exporter state", nil, nil)
	tcpMutex       = &sync.Mutex{}
)

// TCP prom
type TCP struct {
	Monitor *monitor.TCPPort
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
	tcpMutex.Lock()
	defer tcpMutex.Unlock()

	if m := p.Monitor.ExportMetrics(); len(m) > 0 {
		p.metrics = m
	}

	if l := p.Monitor.ExportLabels(); len(l) > 0 {
		p.labels = l
	}

	if len(p.metrics) > 0 {
		ch <- prometheus.MustNewConstMetric(tcpStateDesc, prometheus.GaugeValue, 1)
	} else {
		ch <- prometheus.MustNewConstMetric(tcpStateDesc, prometheus.GaugeValue, 0)
	}

	targets := []string{}
	for target, metric := range p.metrics {
		targets = append(targets, target)
		l := strings.SplitN(strings.SplitN(target, " ", 2)[0], " ", 2) // get name without ip and create slice
		l = append(l, metric.DestAddr)
		l = append(l, metric.DestIp)
		l = append(l, metric.DestPort)
		l2 := prometheus.Labels(p.labels[target])

		tcpTimeDesc = prometheus.NewDesc("tcp_connection_seconds", "Connection time in seconds", tcpLabelNames, l2)
		tcpStatusDesc = prometheus.NewDesc("tcp_connection_status", "Connection Status", tcpLabelNames, l2)

		ch <- prometheus.MustNewConstMetric(tcpTimeDesc, prometheus.GaugeValue, metric.ConTime.Seconds(), l...)

		if metric.Success {
			ch <- prometheus.MustNewConstMetric(tcpStatusDesc, prometheus.GaugeValue, 1, l...)
		} else {
			ch <- prometheus.MustNewConstMetric(tcpStatusDesc, prometheus.GaugeValue, 0, l...)
		}
	}
	ch <- prometheus.MustNewConstMetric(tcpTargetsDesc, prometheus.GaugeValue, float64(len(targets)))
}
