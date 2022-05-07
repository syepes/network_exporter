package collector

import (
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/syepes/network_exporter/monitor"
	"github.com/syepes/network_exporter/pkg/ping"
)

var (
	icmpLabelNames  = []string{"name", "target", "target_ip"}
	icmpStatusDesc  = prometheus.NewDesc("ping_status", "Ping Status", icmpLabelNames, nil)
	icmpRttDesc     = prometheus.NewDesc("ping_rtt_seconds", "Round Trip Time in seconds", append(icmpLabelNames, "type"), nil)
	icmpLossDesc    = prometheus.NewDesc("ping_loss_percent", "Packet loss in percent", icmpLabelNames, nil)
	icmpTargetsDesc = prometheus.NewDesc("ping_targets", "Number of active targets", nil, nil)
	icmpStateDesc   = prometheus.NewDesc("ping_up", "Exporter state", nil, nil)
	icmpMutex       = &sync.Mutex{}
)

// PING prom
type PING struct {
	Monitor *monitor.PING
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
	icmpMutex.Lock()
	defer icmpMutex.Unlock()

	if m := p.Monitor.ExportMetrics(); len(m) > 0 {
		p.metrics = m
	}

	if l := p.Monitor.ExportLabels(); len(l) > 0 {
		p.labels = l
	}

	if len(p.metrics) > 0 {
		ch <- prometheus.MustNewConstMetric(icmpStateDesc, prometheus.GaugeValue, 1)
	} else {
		ch <- prometheus.MustNewConstMetric(icmpStateDesc, prometheus.GaugeValue, 0)
	}

	targets := []string{}
	for target, metric := range p.metrics {
		targets = append(targets, target)
		l := strings.SplitN(strings.SplitN(target, " ", 2)[0], " ", 2) // get name without ip and create slice
		l = append(l, metric.DestAddr)
		l = append(l, metric.DestIp)
		l2 := prometheus.Labels(p.labels[target])

		icmpStatusDesc = prometheus.NewDesc("ping_status", "Ping Status", icmpLabelNames, l2)
		icmpRttDesc = prometheus.NewDesc("ping_rtt_seconds", "Round Trip Time in seconds", append(icmpLabelNames, "type"), l2)
		icmpLossDesc = prometheus.NewDesc("ping_loss_percent", "Packet loss in percent", icmpLabelNames, l2)

		if metric.Success {
			ch <- prometheus.MustNewConstMetric(icmpStatusDesc, prometheus.GaugeValue, 1, l...)
		} else {
			ch <- prometheus.MustNewConstMetric(icmpStatusDesc, prometheus.GaugeValue, 0, l...)
		}

		ch <- prometheus.MustNewConstMetric(icmpRttDesc, prometheus.GaugeValue, metric.BestTime.Seconds(), append(l, "best")...)
		ch <- prometheus.MustNewConstMetric(icmpRttDesc, prometheus.GaugeValue, metric.AvgTime.Seconds(), append(l, "mean")...)
		ch <- prometheus.MustNewConstMetric(icmpRttDesc, prometheus.GaugeValue, metric.WorstTime.Seconds(), append(l, "worst")...)
		ch <- prometheus.MustNewConstMetric(icmpRttDesc, prometheus.GaugeValue, metric.SumTime.Seconds(), append(l, "sum")...)
		ch <- prometheus.MustNewConstMetric(icmpRttDesc, prometheus.GaugeValue, metric.SquaredDeviationTime.Seconds(), append(l, "sd")...)
		ch <- prometheus.MustNewConstMetric(icmpRttDesc, prometheus.GaugeValue, metric.UncorrectedSDTime.Seconds(), append(l, "usd")...)
		ch <- prometheus.MustNewConstMetric(icmpRttDesc, prometheus.GaugeValue, metric.CorrectedSDTime.Seconds(), append(l, "csd")...)
		ch <- prometheus.MustNewConstMetric(icmpRttDesc, prometheus.GaugeValue, metric.RangeTime.Seconds(), append(l, "range")...)
		ch <- prometheus.MustNewConstMetric(icmpLossDesc, prometheus.GaugeValue, metric.DropRate, l...)
	}
	ch <- prometheus.MustNewConstMetric(icmpTargetsDesc, prometheus.GaugeValue, float64(len(targets)))
}
