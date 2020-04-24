package collector

import (
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/syepes/ping_exporter/monitor"
	"github.com/syepes/ping_exporter/pkg/mtr"
)

var (
	mtrLabelNames  = []string{"name", "target", "ttl", "path"}
	mtrDesc        = prometheus.NewDesc("mtr_rtt_seconds", "Round Trip Time in seconds", append(mtrLabelNames, "type"), nil)
	mtrHopsDesc    = prometheus.NewDesc("mtr_hops", "Number of route hops", nil, nil)
	mtrTargetsDesc = prometheus.NewDesc("mtr_targets", "Number of active targets", nil, nil)
	mtrStateDesc   = prometheus.NewDesc("mtr_up", "Exporter state", nil, nil)
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
	ch <- mtrHopsDesc
	ch <- mtrTargetsDesc
	ch <- mtrStateDesc
}

// Collect prom
func (p *MTR) Collect(ch chan<- prometheus.Metric) {
	mtrMutex.Lock()
	defer mtrMutex.Unlock()

	if m := p.Monitor.Export(); len(m) > 0 {
		p.metrics = m
	}

	if len(p.metrics) > 0 {
		ch <- prometheus.MustNewConstMetric(mtrStateDesc, prometheus.GaugeValue, 1)
	} else {
		ch <- prometheus.MustNewConstMetric(mtrStateDesc, prometheus.GaugeValue, 0)
	}

	targets := []string{}
	for target, metric := range p.metrics {
		targets = append(targets, target)
		// fmt.Printf("target: %v\n", target)
		// fmt.Printf("metric: %v\n", metric)
		// l := strings.SplitN(target, " ", 2)
		l := []string{target, metric.DestAddr}
		// fmt.Printf("L: %v\n", l)

		ch <- prometheus.MustNewConstMetric(mtrHopsDesc, prometheus.GaugeValue, float64(len(metric.Hops)))
		for _, hop := range metric.Hops {
			ll := append(l, strconv.Itoa(hop.TTL))
			ll = append(ll, hop.AddressTo)
			// fmt.Printf("LL: %v\n", ll)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, hop.LastTime.Seconds(), append(ll, "last")...)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, hop.SumTime.Seconds(), append(ll, "sum")...)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, hop.BestTime.Seconds(), append(ll, "best")...)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, hop.AvgTime.Seconds(), append(ll, "mean")...)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, hop.WorstTime.Seconds(), append(ll, "worst")...)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, hop.StdDev.Seconds(), append(ll, "stddev")...)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, float64(hop.Loss), append(ll, "loss")...)
		}
	}
	ch <- prometheus.MustNewConstMetric(mtrTargetsDesc, prometheus.GaugeValue, float64(len(targets)))
}
