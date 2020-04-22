package collector

import (
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/syepes/ping_exporter/monitor"
	"github.com/syepes/ping_exporter/pkg/mtr"
)

var (
	// mtrLabelNames = []string{"name", "target", "ip", "ip_version"}
	mtrLabelNames  = []string{"name", "target", "ttl", "path"}
	mtrDesc        = prometheus.NewDesc("mtr_rtt_seconds", "MTR Round trip time in seconds", append(mtrLabelNames, "type"), nil)
	mtrTargetsDesc = prometheus.NewDesc("mtr_targets", "Number of active targets", nil, nil)
	mtrProgDesc    = prometheus.NewDesc("mtr_up", "ping_exporter version", nil, nil)
	mtrMutex       = &sync.Mutex{}
)

// MTR prom
type MTR struct {
	Monitor *monitor.MTR
	metrics map[string]*mtr.MtrResult
}

// Describe prom
func (p *MTR) Describe(ch chan<- *prometheus.Desc) {
	ch <- mtrDesc
	ch <- mtrProgDesc
}

// Collect prom
func (p *MTR) Collect(ch chan<- prometheus.Metric) {
	mtrMutex.Lock()
	defer mtrMutex.Unlock()

	if m := p.Monitor.Export(); len(m) > 0 {
		p.metrics = m
	}

	if len(p.metrics) > 0 {
		ch <- prometheus.MustNewConstMetric(mtrProgDesc, prometheus.GaugeValue, 1)
	} else {
		ch <- prometheus.MustNewConstMetric(mtrProgDesc, prometheus.GaugeValue, 0)
	}

	targets := []string{}
	for target, metrics := range p.metrics {
		// fmt.Printf("target: %v\n", target)
		// fmt.Printf("metrics: %v\n", metrics)
		// l := strings.SplitN(target, " ", 2)
		targets = append(targets, target)
		l := []string{target, metrics.DestAddress}
		// fmt.Printf("L: %v\n", l)
		for _, hop := range metrics.Hops {
			ll := append(l, strconv.Itoa(hop.TTL))
			ll = append(ll, hop.AddressTo)
			// fmt.Printf("LL: %v\n", ll)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, float64(hop.LastTime.Seconds()), append(ll, "last")...)
			// ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, float64(hop.BestTime.Seconds()), append(ll, "best")...)
			// ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, float64(hop.AvgTime.Seconds()), append(ll, "mean")...)
			// ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, float64(hop.WrstTime.Seconds()), append(ll, "worst")...)
			// ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, float64(hop.Loss), append(ll, "loss")...)
		}
	}
	ch <- prometheus.MustNewConstMetric(mtrTargetsDesc, prometheus.GaugeValue, float64(len(targets)))
}
