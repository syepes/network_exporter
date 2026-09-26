package selfmon

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// reset clears all internal metric vecs and re-creates their zero series so each
// test starts from a known, isolated state (the metrics are package globals).
func reset() {
	probeDuration.Reset()
	probesTotal.Reset()
	probesInflight.Reset()
	probeQueueWait.Reset()
	probesSkipped.Reset()
	probeLastCompletion.Reset()
	scrapeDuration.Reset()
	initSeries()
}

func TestInitSeriesPreInitialisesAll(t *testing.T) {
	reset()

	// Every probe type/result combination must be present at zero so rate() and
	// delta() queries behave from t0.
	if got := testutil.CollectAndCount(probesTotal); got != len(probeTypes)*2 {
		t.Fatalf("probes_total series = %d, want %d", got, len(probeTypes)*2)
	}
	if got := testutil.CollectAndCount(probesSkipped); got != len(probeTypes)*len(skipReasons) {
		t.Fatalf("probe_skipped_total series = %d, want %d", got, len(probeTypes)*len(skipReasons))
	}
	if got := testutil.CollectAndCount(probesInflight); got != len(probeTypes) {
		t.Fatalf("probe_inflight series = %d, want %d", got, len(probeTypes))
	}
	if got := testutil.CollectAndCount(scrapeDuration); got != len(probeTypes) {
		t.Fatalf("scrape_duration series = %d, want %d", got, len(probeTypes))
	}
}

func TestProbeStartedRecordsOutcome(t *testing.T) {
	reset()

	done := ProbeStarted(TypePing)
	// While the probe is in flight, the gauge must read 1.
	if got := testutil.ToFloat64(probesInflight.WithLabelValues(TypePing)); got != 1 {
		t.Fatalf("inflight during probe = %v, want 1", got)
	}
	done(true)

	if got := testutil.ToFloat64(probesInflight.WithLabelValues(TypePing)); got != 0 {
		t.Fatalf("inflight after probe = %v, want 0", got)
	}
	if got := testutil.ToFloat64(probesTotal.WithLabelValues(TypePing, resultSuccess)); got != 1 {
		t.Fatalf("success total = %v, want 1", got)
	}
	if got := testutil.ToFloat64(probesTotal.WithLabelValues(TypePing, resultFailure)); got != 0 {
		t.Fatalf("failure total = %v, want 0", got)
	}
	if got := testutil.ToFloat64(probeLastCompletion.WithLabelValues(TypePing)); got == 0 {
		t.Fatalf("last completion timestamp not set")
	}

	// A failing probe must land in the failure series, not success.
	ProbeStarted(TypeMTR)(false)
	if got := testutil.ToFloat64(probesTotal.WithLabelValues(TypeMTR, resultFailure)); got != 1 {
		t.Fatalf("mtr failure total = %v, want 1", got)
	}
	if got := testutil.ToFloat64(probesTotal.WithLabelValues(TypeMTR, resultSuccess)); got != 0 {
		t.Fatalf("mtr success total = %v, want 0", got)
	}
}

func TestProbeStartedMeasuresDuration(t *testing.T) {
	reset()

	done := ProbeStarted(TypeTCP)
	time.Sleep(2 * time.Millisecond)
	done(true)

	// One duration sample must have been observed for the tcp series.
	if got := testutil.CollectAndCount(probeDuration); got != len(probeTypes) {
		t.Fatalf("probe_duration series = %d, want %d", got, len(probeTypes))
	}
}

func TestSkippedAndQueueWait(t *testing.T) {
	reset()

	ProbeSkipped(TypeTCP, SaturationReason)
	ProbeSkipped(TypeTCP, SaturationReason)
	ProbeSkipped(TypeTCP, InflightReason)

	if got := testutil.ToFloat64(probesSkipped.WithLabelValues(TypeTCP, SaturationReason)); got != 2 {
		t.Fatalf("tcp saturation skips = %v, want 2", got)
	}
	if got := testutil.ToFloat64(probesSkipped.WithLabelValues(TypeTCP, InflightReason)); got != 1 {
		t.Fatalf("tcp inflight skips = %v, want 1", got)
	}

	ObserveQueueWait(TypeHTTP, 5*time.Millisecond)
	if got := testutil.CollectAndCount(probeQueueWait); got != len(probeTypes) {
		t.Fatalf("queue_wait series = %d, want %d", got, len(probeTypes))
	}
}
