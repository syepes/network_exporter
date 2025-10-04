package collector

import (
	"fmt"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/syepes/network_exporter/monitor"
	"github.com/syepes/network_exporter/pkg/ping"
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
	// Descriptor cache for custom labels
	icmpDescCache      = make(map[string]*descriptorSet)
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

// getDescriptors returns cached or creates new descriptors for a label set
func getDescriptors(labels prometheus.Labels) *descriptorSet {
	// Create cache key from labels
	cacheKey := fmt.Sprintf("%v", labels)

	// Try read lock first
	icmpDescCacheMutex.RLock()
	if descSet, exists := icmpDescCache[cacheKey]; exists {
		icmpDescCacheMutex.RUnlock()
		return descSet
	}
	icmpDescCacheMutex.RUnlock()

	// Create new descriptor set
	icmpDescCacheMutex.Lock()
	defer icmpDescCacheMutex.Unlock()

	// Double-check after acquiring write lock
	if descSet, exists := icmpDescCache[cacheKey]; exists {
		return descSet
	}

	descSet := &descriptorSet{
		status:         prometheus.NewDesc("ping_status", "Ping Status", icmpLabelNames, labels),
		rtt:            prometheus.NewDesc("ping_rtt_seconds", "Round Trip Time in seconds", append(icmpLabelNames, "type"), labels),
		sntSummary:     prometheus.NewDesc("ping_rtt_snt_count", "Packet sent count", icmpLabelNames, labels),
		sntFailSummary: prometheus.NewDesc("ping_rtt_snt_fail_count", "Packet sent fail count", icmpLabelNames, labels),
		sntTimeSummary: prometheus.NewDesc("ping_rtt_snt_seconds", "Packet sent time total", icmpLabelNames, labels),
		loss:           prometheus.NewDesc("ping_loss_percent", "Packet loss in percent", icmpLabelNames, labels),
	}
	icmpDescCache[cacheKey] = descSet
	return descSet
}

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

		// Get cached descriptors for this label set
		descs := getDescriptors(l2)

		if metric.Success {
			ch <- prometheus.MustNewConstMetric(descs.status, prometheus.GaugeValue, 1, l...)
		} else {
			ch <- prometheus.MustNewConstMetric(descs.status, prometheus.GaugeValue, 0, l...)
		}

		ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, metric.BestTime.Seconds(), append(l, "best")...)
		ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, metric.AvgTime.Seconds(), append(l, "mean")...)
		ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, metric.WorstTime.Seconds(), append(l, "worst")...)
		ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, metric.SumTime.Seconds(), append(l, "sum")...)
		ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, metric.SquaredDeviationTime.Seconds(), append(l, "sd")...)
		ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, metric.UncorrectedSDTime.Seconds(), append(l, "usd")...)
		ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, metric.CorrectedSDTime.Seconds(), append(l, "csd")...)
		ch <- prometheus.MustNewConstMetric(descs.rtt, prometheus.GaugeValue, metric.RangeTime.Seconds(), append(l, "range")...)
		ch <- prometheus.MustNewConstMetric(descs.sntSummary, prometheus.GaugeValue, float64(metric.SntSummary), l...)
		ch <- prometheus.MustNewConstMetric(descs.sntFailSummary, prometheus.GaugeValue, float64(metric.SntFailSummary), l...)
		ch <- prometheus.MustNewConstMetric(descs.sntTimeSummary, prometheus.GaugeValue, metric.SntTimeSummary.Seconds(), l...)
		ch <- prometheus.MustNewConstMetric(descs.loss, prometheus.GaugeValue, metric.DropRate, l...)
	}
	ch <- prometheus.MustNewConstMetric(icmpTargetsDesc, prometheus.GaugeValue, float64(len(targets)))
}
