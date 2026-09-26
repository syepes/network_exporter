package collector

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/syepes/network_exporter/pkg/common"
	"github.com/syepes/network_exporter/pkg/ping"
	"github.com/syepes/network_exporter/pkg/tcp"
)

// gatherCount registers c on a normal (non-pedantic) registry, mirroring how
// main.go wires the collectors, and returns the number of series with the given
// metric name currently exposed on /metrics.
func gatherCount(t *testing.T, c prometheus.Collector, metricName string) int {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	n, err := testutil.GatherAndCount(reg, metricName)
	if err != nil {
		t.Fatalf("gather %q: %v", metricName, err)
	}
	return n
}

// fakePingMonitor is a stand-in for *monitor.PING that lets tests drive the
// exported metrics/labels and the live target set independently, exactly as a
// SIGHUP config reload would change them at runtime.
type fakePingMonitor struct {
	metrics map[string]*ping.PingResult
	labels  map[string]map[string]string
	names   []string
}

func (f *fakePingMonitor) Snapshot() common.Snapshot[ping.PingResult] {
	return common.Snapshot[ping.PingResult]{Metrics: f.metrics, Labels: f.labels, Names: f.names}
}

func pingTarget(name string) (string, *ping.PingResult, map[string]string) {
	key := name + " 1.1.1.1"
	return key, &ping.PingResult{Success: true, DestAddr: "1.1.1.1", DestIp: "1.1.1.1"}, map[string]string{}
}

func (f *fakePingMonitor) set(names ...string) {
	f.metrics = map[string]*ping.PingResult{}
	f.labels = map[string]map[string]string{}
	f.names = nil
	for _, n := range names {
		key, res, lbl := pingTarget(n)
		f.metrics[key] = res
		f.labels[key] = lbl
		f.names = append(f.names, key)
	}
}

// TestPingCollectorReflectsTargetChanges exercises the full Collector path
// (Collect -> /metrics exposition) and asserts that adding and removing targets
// at runtime is reflected in the exported series, which is what a SIGHUP reload
// ultimately drives.
func TestPingCollectorReflectsTargetChanges(t *testing.T) {
	fm := &fakePingMonitor{}
	c := &PING{Monitor: fm}

	// Start with two targets.
	fm.set("host1", "host2")
	if got := gatherCount(t, c, "ping_status"); got != 2 {
		t.Fatalf("initial: expected 2 ping_status series, got %d", got)
	}

	// Add a third target.
	fm.set("host1", "host2", "host3")
	if got := gatherCount(t, c, "ping_status"); got != 3 {
		t.Fatalf("after add: expected 3 ping_status series, got %d", got)
	}

	// Remove one target.
	fm.set("host1", "host3")
	if got := gatherCount(t, c, "ping_status"); got != 2 {
		t.Fatalf("after partial removal: expected 2 ping_status series, got %d", got)
	}

	// Remove every target (the regression case): nothing must remain exported.
	fm.set()
	if got := gatherCount(t, c, "ping_status"); got != 0 {
		t.Fatalf("after full removal: expected 0 ping_status series, got %d", got)
	}

	expected := `
# HELP ping_targets Number of active targets
# TYPE ping_targets gauge
ping_targets 0
# HELP ping_up Exporter state
# TYPE ping_up gauge
ping_up 0
`
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	if err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "ping_targets", "ping_up"); err != nil {
		t.Fatalf("after full removal, ping_targets/ping_up mismatch: %v", err)
	}
}

// TestPingCollectorKeepsPendingTargets verifies that a target which is still
// active but has not produced a fresh result this scrape keeps its previously
// exported value, instead of flapping to absent. This is the transient-gap
// smoothing the reconciliation must preserve.
func TestPingCollectorKeepsPendingTargets(t *testing.T) {
	fm := &fakePingMonitor{}
	c := &PING{Monitor: fm}

	fm.set("host1")
	if got := gatherCount(t, c, "ping_status"); got != 1 {
		t.Fatalf("initial: expected 1 ping_status series, got %d", got)
	}

	// Target is still configured/active but produced no result this scrape.
	key, _, _ := pingTarget("host1")
	fm.metrics = map[string]*ping.PingResult{}
	fm.labels = map[string]map[string]string{}
	fm.names = []string{key}

	if got := gatherCount(t, c, "ping_status"); got != 1 {
		t.Fatalf("pending target: expected cached value to persist (1 series), got %d", got)
	}
}

// fakeTCPMonitor mirrors fakePingMonitor for the TCP collector, proving the
// reconciliation fix is consistent across metric types.
type fakeTCPMonitor struct {
	metrics map[string]*tcp.TCPPortReturn
	labels  map[string]map[string]string
	names   []string
}

func (f *fakeTCPMonitor) Snapshot() common.Snapshot[tcp.TCPPortReturn] {
	return common.Snapshot[tcp.TCPPortReturn]{Metrics: f.metrics, Labels: f.labels, Names: f.names}
}

func (f *fakeTCPMonitor) set(names ...string) {
	f.metrics = map[string]*tcp.TCPPortReturn{}
	f.labels = map[string]map[string]string{}
	f.names = nil
	for _, n := range names {
		key := n + " 1.1.1.1"
		f.metrics[key] = &tcp.TCPPortReturn{Success: true, DestAddr: "1.1.1.1", DestIp: "1.1.1.1", DestPort: "80", SrcIp: "0.0.0.0"}
		f.labels[key] = map[string]string{}
		f.names = append(f.names, key)
	}
}

func TestTCPCollectorReflectsTargetChanges(t *testing.T) {
	fm := &fakeTCPMonitor{}
	c := &TCP{Monitor: fm}

	fm.set("host1", "host2")
	if got := gatherCount(t, c, "tcp_connection_status"); got != 2 {
		t.Fatalf("initial: expected 2 tcp_connection_status series, got %d", got)
	}

	fm.set()
	if got := gatherCount(t, c, "tcp_connection_status"); got != 0 {
		t.Fatalf("after full removal: expected 0 tcp_connection_status series, got %d", got)
	}
}

func TestReconcile(t *testing.T) {
	set := func(names ...string) map[string]struct{} { return newActiveSet(names) }

	t.Run("adds fresh entries into a nil cache", func(t *testing.T) {
		got := reconcile[int](nil, map[string]int{"a": 1, "b": 2}, set("a", "b"))
		if len(got) != 2 || got["a"] != 1 || got["b"] != 2 {
			t.Fatalf("unexpected cache: %v", got)
		}
	})

	t.Run("prunes entries no longer active", func(t *testing.T) {
		cache := map[string]int{"a": 1, "b": 2, "c": 3}
		got := reconcile(cache, map[string]int{"a": 10}, set("a", "b"))
		if _, ok := got["c"]; ok {
			t.Fatalf("expected removed target c to be pruned: %v", got)
		}
		if got["a"] != 10 {
			t.Fatalf("expected a to be refreshed to 10, got %d", got["a"])
		}
		if got["b"] != 2 {
			t.Fatalf("expected still-active b to keep cached value 2, got %d", got["b"])
		}
	})

	t.Run("prunes everything when no targets are active", func(t *testing.T) {
		cache := map[string]int{"a": 1, "b": 2}
		got := reconcile(cache, map[string]int{}, set())
		if len(got) != 0 {
			t.Fatalf("expected empty cache after all targets removed, got %v", got)
		}
	})

	t.Run("keeps active target with no fresh value", func(t *testing.T) {
		cache := map[string]int{"a": 1}
		got := reconcile(cache, map[string]int{}, set("a"))
		if got["a"] != 1 {
			t.Fatalf("expected pending target a to keep cached value 1, got %v", got)
		}
	})

	t.Run("swap removes old and skips uncomputed new", func(t *testing.T) {
		cache := map[string]int{"a": 1}
		// "a" removed, "c" added but not yet computed (absent from fresh).
		got := reconcile(cache, map[string]int{}, set("c"))
		if _, ok := got["a"]; ok {
			t.Fatalf("expected removed target a to be pruned: %v", got)
		}
		if _, ok := got["c"]; ok {
			t.Fatalf("expected uncomputed target c to stay absent until it produces a value: %v", got)
		}
	})
}
