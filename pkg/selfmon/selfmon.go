// Package selfmon exposes internal ("self monitoring") Prometheus metrics that
// describe how the exporter itself is scheduling and executing its network
// checks, as opposed to the results of those checks.
//
// These metrics are meant to answer operational questions that the per-target
// probe results cannot: are probes keeping up with their configured interval, is
// per-target or global concurrency saturated, how long does a probe take to run,
// how often do probes fail to execute, and how expensive is a /metrics scrape.
// They are the primary signal for spotting scheduling and execution bottlenecks
// as the number of active targets grows.
//
// All series are pre-initialised in Register so that they are present (at zero)
// on the very first scrape, which keeps rate() and delta() queries well behaved.
package selfmon

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Namespace is the common prefix for every internal metric.
const Namespace = "network_exporter"

// Probe type label values. These identify the kind of check and are stable
// across the current per-target scheduler and the future global scheduler.
const (
	TypePing = "ping"
	TypeMTR  = "mtr"
	TypeTCP  = "tcp"
	TypeHTTP = "http"
)

// Probe result label values for probesTotal.
const (
	resultSuccess = "success"
	resultFailure = "failure"
)

// Skip reason label values for probesSkipped. SaturationReason is recorded when
// a probe cannot start because the concurrency budget is exhausted;
// InflightReason when a previous run of the same target is still executing.
const (
	SaturationReason = "saturation"
	InflightReason   = "inflight"
)

// probeTypes and skipReasons drive pre-initialisation of the label combinations.
var (
	probeTypes  = []string{TypePing, TypeMTR, TypeTCP, TypeHTTP}
	skipReasons = []string{SaturationReason, InflightReason}
)

var (
	probeDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: Namespace,
		Subsystem: "probe",
		Name:      "duration_seconds",
		Help:      "Wall-clock duration of a single probe execution in seconds, by probe type.",
		// ~1ms to ~32s, covering fast local checks up to long MTR/timeouts.
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 16),
	}, []string{"type"})

	probesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Subsystem: "probe",
		Name:      "total",
		Help:      "Total number of probe executions, by probe type and execution result (success means the probe ran without an execution error, independent of target reachability).",
	}, []string{"type", "result"})

	probesInflight = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Subsystem: "probe",
		Name:      "inflight",
		Help:      "Number of probes currently executing, by probe type.",
	}, []string{"type"})

	probeQueueWait = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: Namespace,
		Subsystem: "probe",
		Name:      "queue_wait_seconds",
		Help:      "Time a scheduled probe waited to acquire a concurrency slot before executing, by probe type. Sustained non-zero values indicate a scheduling/concurrency bottleneck.",
		Buckets:   prometheus.ExponentialBuckets(0.0005, 2, 16),
	}, []string{"type"})

	probesSkipped = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Subsystem: "probe",
		Name:      "skipped_total",
		Help:      "Total number of probe cycles skipped instead of executed, by probe type and reason (saturation: concurrency budget exhausted; inflight: previous run still executing).",
	}, []string{"type", "reason"})

	probeLastCompletion = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Subsystem: "probe",
		Name:      "last_completion_timestamp_seconds",
		Help:      "Unix timestamp of the most recent completed probe, by probe type. A value that stops advancing indicates the scheduler has stalled for that type.",
	}, []string{"type"})

	scrapeDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: Namespace,
		Subsystem: "collector",
		Name:      "scrape_duration_seconds",
		Help:      "Duration of a single collector Collect() call in seconds, by collector. Rising values indicate the /metrics scrape path is becoming a bottleneck.",
		Buckets:   prometheus.ExponentialBuckets(0.0005, 2, 16),
	}, []string{"collector"})
)

// Register registers every internal metric with r and pre-initialises all known
// label combinations to zero so they appear on the first scrape.
func Register(r prometheus.Registerer) {
	r.MustRegister(
		probeDuration,
		probesTotal,
		probesInflight,
		probeQueueWait,
		probesSkipped,
		probeLastCompletion,
		scrapeDuration,
	)
	initSeries()
}

// initSeries pre-creates every known label combination at its zero value.
func initSeries() {
	for _, t := range probeTypes {
		probeDuration.WithLabelValues(t)
		probesTotal.WithLabelValues(t, resultSuccess)
		probesTotal.WithLabelValues(t, resultFailure)
		probesInflight.WithLabelValues(t)
		probeQueueWait.WithLabelValues(t)
		probeLastCompletion.WithLabelValues(t)
		scrapeDuration.WithLabelValues(t)
		for _, reason := range skipReasons {
			probesSkipped.WithLabelValues(t, reason)
		}
	}
}

// ProbeStarted marks a probe of the given type as executing. The returned
// function must be called exactly once when the probe finishes (defer it): it
// records the execution duration, the success/failure outcome, decrements the
// in-flight gauge, and stamps the last-completion time.
func ProbeStarted(typ string) (done func(success bool)) {
	start := time.Now()
	probesInflight.WithLabelValues(typ).Inc()
	return func(success bool) {
		probesInflight.WithLabelValues(typ).Dec()
		probeDuration.WithLabelValues(typ).Observe(time.Since(start).Seconds())
		result := resultSuccess
		if !success {
			result = resultFailure
		}
		probesTotal.WithLabelValues(typ, result).Inc()
		probeLastCompletion.WithLabelValues(typ).Set(float64(time.Now().Unix()))
	}
}

// ObserveQueueWait records how long a scheduled probe waited for a concurrency
// slot before it could start executing.
func ObserveQueueWait(typ string, d time.Duration) {
	probeQueueWait.WithLabelValues(typ).Observe(d.Seconds())
}

// ProbeSkipped records that a probe cycle was skipped instead of executed.
func ProbeSkipped(typ string, reason string) {
	probesSkipped.WithLabelValues(typ, reason).Inc()
}

// ObserveScrape records the duration of a collector Collect() call. Pass the
// time captured at the start of Collect; typically used as:
//
//	defer selfmon.ObserveScrape("ping", time.Now())
func ObserveScrape(collector string, start time.Time) {
	scrapeDuration.WithLabelValues(collector).Observe(time.Since(start).Seconds())
}
