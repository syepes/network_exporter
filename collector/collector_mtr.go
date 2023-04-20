package collector

import (
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/syepes/network_exporter/monitor"
	"github.com/syepes/network_exporter/pkg/mtr"
)

var (
	mtrLabelNames  = []string{"name", "target", "ttl", "path"}
	mtrDesc        = prometheus.NewDesc("mtr_rtt_seconds", "Round Trip Time in seconds", append(mtrLabelNames, "type"), nil)
	mtrSntDesc     = prometheus.NewDesc("mtr_rtt_snt_count", "Round Trip Send Package Total", append(mtrLabelNames, "type"), nil)
	mtrSntFailDesc = prometheus.NewDesc("mtr_rtt_snt_fail_count", "Round Trip Send Package Fail Total", append(mtrLabelNames, "type"), nil)
	mtrSntTimeDesc = prometheus.NewDesc("mtr_rtt_snt_seconds", "Round Trip Send Package Time Total", append(mtrLabelNames, "type"), nil)
	mtrHopsDesc    = prometheus.NewDesc("mtr_hops", "Number of route hops", []string{"name", "target"}, nil)
	mtrTargetsDesc = prometheus.NewDesc("mtr_targets", "Number of active targets", nil, nil)
	mtrStateDesc   = prometheus.NewDesc("mtr_up", "Exporter state", nil, nil)
	mtrMutex       = &sync.Mutex{}
)

// MTR prom
type MTR struct {
	Monitor *monitor.MTR
	metrics map[string]*mtr.MtrResult
	labels  map[string]map[string]string
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

	if m := p.Monitor.ExportMetrics(); len(m) > 0 {
		p.metrics = m
	}

	if l := p.Monitor.ExportLabels(); len(l) > 0 {
		p.labels = l
	}

	if len(p.metrics) > 0 {
		ch <- prometheus.MustNewConstMetric(mtrStateDesc, prometheus.GaugeValue, 1)
	} else {
		ch <- prometheus.MustNewConstMetric(mtrStateDesc, prometheus.GaugeValue, 0)
	}

	targets := []string{}
	for target, metric := range p.metrics {
		targets = append(targets, target)
		l := []string{target, metric.DestAddr}
		l2 := prometheus.Labels(p.labels[target])

		mtrDesc = prometheus.NewDesc("mtr_rtt_seconds", "Round Trip Time in seconds", append(mtrLabelNames, "type"), l2)
		mtrHopsDesc = prometheus.NewDesc("mtr_hops", "Number of route hops", []string{"name", "target"}, l2)

		ch <- prometheus.MustNewConstMetric(mtrHopsDesc, prometheus.GaugeValue, float64(len(metric.Hops)), l...)
		for _, hop := range metric.Hops {
			ll := append(l, strconv.Itoa(hop.TTL))
			ll = append(ll, hop.AddressTo)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, hop.LastTime.Seconds(), append(ll, "last")...)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, hop.SumTime.Seconds(), append(ll, "sum")...)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, hop.BestTime.Seconds(), append(ll, "best")...)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, hop.AvgTime.Seconds(), append(ll, "mean")...)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, hop.WorstTime.Seconds(), append(ll, "worst")...)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, hop.SquaredDeviationTime.Seconds(), append(ll, "sd")...)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, hop.UncorrectedSDTime.Seconds(), append(ll, "usd")...)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, hop.CorrectedSDTime.Seconds(), append(ll, "csd")...)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, hop.RangeTime.Seconds(), append(ll, "range")...)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, float64(hop.Loss), append(ll, "loss")...)
		}

		mtrSntDesc = prometheus.NewDesc("mtr_rtt_snt_count", "Round Trip Send Package Total", mtrLabelNames, l2)
		mtrSntFailDesc = prometheus.NewDesc("mtr_rtt_snt_fail_count", "Round Trip Send Package Fail Total", mtrLabelNames, l2)
		mtrSntTimeDesc = prometheus.NewDesc("mtr_rtt_snt_seconds", "Round Trip Send Package Time Total", mtrLabelNames, l2)

		for ttl, summary := range metric.HopSummaryMap {
			ll := append(l, strings.Split(ttl, "_")[0])
			ll = append(ll, summary.AddressTo)
			ch <- prometheus.MustNewConstMetric(mtrSntDesc, prometheus.CounterValue, float64(summary.Snt), ll...)
			ch <- prometheus.MustNewConstMetric(mtrSntFailDesc, prometheus.CounterValue, float64(summary.SntFail), ll...)
			ch <- prometheus.MustNewConstMetric(mtrSntTimeDesc, prometheus.CounterValue, summary.SntTime.Seconds(), ll...)
		}
	}
	ch <- prometheus.MustNewConstMetric(mtrTargetsDesc, prometheus.GaugeValue, float64(len(targets)))
}
