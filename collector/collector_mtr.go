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
	mtrLabelNames = []string{"name", "target", "ttl", "path"}
	mtrDesc       = prometheus.NewDesc("mtr_rtt_seconds", "MTR Round trip time in seconds", append(mtrLabelNames, "type"), nil)
	mtrProgDesc   = prometheus.NewDesc("mtr_up", "ping_exporter version", nil, prometheus.Labels{"version": "xzy"})
	mtrMutex      = &sync.Mutex{}
)

// MtrCollector prom
type MtrCollector struct {
	Monitor *monitor.MTR
	metrics map[string]*mtr.MtrResult
}

// Describe prom
func (p *MtrCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- mtrDesc
	ch <- mtrProgDesc
}

// Collect prom
func (p *MtrCollector) Collect(ch chan<- prometheus.Metric) {
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

	for target, metrics := range p.metrics {
		// fmt.Printf("target: %v\n", target)
		// fmt.Printf("metrics: %v\n", metrics)
		// l := strings.SplitN(target, " ", 2)
		l := []string{target, metrics.DestAddress}
		// fmt.Printf("L: %v\n", l)
		for _, hop := range metrics.Hops {
			ll := append(l, strconv.Itoa(hop.TTL))
			ll = append(ll, hop.Host)
			// fmt.Printf("LL: %v\n", ll)
			ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, float64(hop.LastTime.Seconds()), append(ll, "last")...)
			// ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, float64(hop.BestTime.Seconds()), append(ll, "best")...)
			// ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, float64(hop.AvgTime.Seconds()), append(ll, "mean")...)
			// ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, float64(hop.WrstTime.Seconds()), append(ll, "worst")...)
			// ch <- prometheus.MustNewConstMetric(mtrDesc, prometheus.GaugeValue, float64(hop.Loss), append(ll, "loss")...)
		}
	}
}

/*
type IcmpHop struct {
	Success  bool          `json:"success"`
	Address  string        `json:"address"`
	Host     string        `json:"host"`
	N        int           `json:"n"`
	TTL      int           `json:"ttl"`
	Snt      int           `json:"snt"`
	LastTime time.Duration `json:"last"`
	AvgTime  time.Duration `json:"avg"`
	BestTime time.Duration `json:"best"`
	WrstTime time.Duration `json:"worst"`
	Loss     float32       `json:"loss"`
}*/
