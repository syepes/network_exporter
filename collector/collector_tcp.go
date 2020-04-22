package collector

import (
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/syepes/ping_exporter/monitor"
	"github.com/syepes/ping_exporter/pkg/tcp"
)

var (
	// tcpLabelNames = []string{"name", "target", "ip", "ip_version"}
	tcpLabelNames  = []string{"name", "target", "port"}
	tcpTimeDesc    = prometheus.NewDesc("tcp_connection_seconds", "TCP Connection time in seconds", tcpLabelNames, nil)
	tcStatusDesc   = prometheus.NewDesc("tcp_connection_status", "TCP Connection Status", tcpLabelNames, nil)
	tcpTargetsDesc = prometheus.NewDesc("tcp_targets", "Number of active targets", nil, nil)
	tcpProgDesc    = prometheus.NewDesc("tcp_up", "ping_exporter version", nil, nil)
	tcpMutex       = &sync.Mutex{}
)

// TCP prom
type TCP struct {
	Monitor *monitor.TCPPort
	metrics map[string]*tcp.TCPPortReturn
}

// Describe prom
func (p *TCP) Describe(ch chan<- *prometheus.Desc) {
	ch <- tcpTimeDesc
	ch <- tcStatusDesc
	ch <- tcpTargetsDesc
	ch <- tcpProgDesc
}

// Collect prom
func (p *TCP) Collect(ch chan<- prometheus.Metric) {
	tcpMutex.Lock()
	defer tcpMutex.Unlock()

	if m := p.Monitor.Export(); len(m) > 0 {
		p.metrics = m
	}

	if len(p.metrics) > 0 {
		ch <- prometheus.MustNewConstMetric(tcpProgDesc, prometheus.GaugeValue, 1)
	} else {
		ch <- prometheus.MustNewConstMetric(tcpProgDesc, prometheus.GaugeValue, 0)
	}

	targets := []string{}
	for target, metric := range p.metrics {
		targets = append(targets, target)
		// fmt.Printf("target: %v\n", target)
		// fmt.Printf("metric: %v\n", metric)
		l := strings.SplitN(target, " ", 2)
		l = append(l, metric.DestAddr)
		l = append(l, metric.DestPort)
		// fmt.Printf("L: %v\n", l)

		ch <- prometheus.MustNewConstMetric(tcpTimeDesc, prometheus.GaugeValue, float64(metric.ConTime/1000), l...)

		if metric.Success == true {
			ch <- prometheus.MustNewConstMetric(tcStatusDesc, prometheus.GaugeValue, 1, l...)
		} else {
			ch <- prometheus.MustNewConstMetric(tcStatusDesc, prometheus.GaugeValue, 0, l...)
		}

	}
	ch <- prometheus.MustNewConstMetric(tcpTargetsDesc, prometheus.GaugeValue, float64(len(targets)))
}
