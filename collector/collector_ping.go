package collector

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/syepes/ping_exporter/monitor"
	"github.com/syepes/ping_exporter/pkg/ping"
)

var (
	// icmpLabelNames = []string{"name", "target", "ip", "ip_version"}
	icmpLabelNames  = []string{"name", "target"}
	icmpRttDesc     = prometheus.NewDesc("ping_rtt_seconds", "ICMP Round trip time in seconds", append(icmpLabelNames, "type"), nil)
	icmpLossDesc    = prometheus.NewDesc("ping_loss_percent", "Packet loss in percent", icmpLabelNames, nil)
	icmpTargetsDesc = prometheus.NewDesc("ping_targets", "Number of active targets", nil, nil)
	icmpProgDesc    = prometheus.NewDesc("ping_up", "ping_exporter version", nil, nil)
	icmpMutex       = &sync.Mutex{}
)

// PING prom
type PING struct {
	Monitor *monitor.PING
	metrics map[string]*ping.PingReturn
}

// Describe prom
func (p *PING) Describe(ch chan<- *prometheus.Desc) {
	ch <- icmpRttDesc
	ch <- icmpLossDesc
	ch <- icmpTargetsDesc
	ch <- icmpProgDesc
}

// Collect prom
func (p *PING) Collect(ch chan<- prometheus.Metric) {
	icmpMutex.Lock()
	defer icmpMutex.Unlock()

	if m := p.Monitor.Export(); len(m) > 0 {
		p.metrics = m
	}

	if len(p.metrics) > 0 {
		ch <- prometheus.MustNewConstMetric(icmpProgDesc, prometheus.GaugeValue, 1)
	} else {
		ch <- prometheus.MustNewConstMetric(icmpProgDesc, prometheus.GaugeValue, 0)
	}

	targets := []string{}
	for target, metrics := range p.metrics {
		// fmt.Printf("target: %v\n", target)
		// fmt.Printf("metrics: %v\n", metrics)
		// l := strings.SplitN(target, " ", 2)
		targets = append(targets, target)
		l := []string{target, metrics.DestAddr}
		// fmt.Printf("L: %v\n", l)

		ch <- prometheus.MustNewConstMetric(icmpRttDesc, prometheus.GaugeValue, float64(metrics.BestTime/1000), append(l, "best")...)
		ch <- prometheus.MustNewConstMetric(icmpRttDesc, prometheus.GaugeValue, float64(metrics.AvgTime/1000), append(l, "mean")...)
		ch <- prometheus.MustNewConstMetric(icmpRttDesc, prometheus.GaugeValue, float64(metrics.WrstTime/1000), append(l, "worst")...)
		ch <- prometheus.MustNewConstMetric(icmpRttDesc, prometheus.GaugeValue, float64(metrics.AllTime/1000), append(l, "all")...)
		ch <- prometheus.MustNewConstMetric(icmpLossDesc, prometheus.GaugeValue, metrics.DropRate, l...)
	}
	ch <- prometheus.MustNewConstMetric(icmpTargetsDesc, prometheus.GaugeValue, float64(len(targets)))
}
