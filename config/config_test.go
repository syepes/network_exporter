package config

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempConfig writes body to a temp YAML file and returns its path.
func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "network_exporter.yml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

// TestReloadConfigTTLValidation exercises the icmp.ttl / mtr.first-ttl range and
// ordering checks added for the Cilium-friendly initial-TTL feature. Configs carry
// no targets so the loader stays fully offline (no DNS resolution).
func TestReloadConfigTTLValidation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Base template: everything but the TTL knobs left to defaults. The two %s
	// slots receive the icmp/mtr overrides under test.
	const base = "icmp:\n" +
		"  interval: 3s\n" +
		"  timeout: 1s\n" +
		"%s" +
		"mtr:\n" +
		"  interval: 3s\n" +
		"  timeout: 1s\n" +
		"  max-hops: 30\n" +
		"%s" +
		"tcp:\n" +
		"  interval: 3s\n" +
		"  timeout: 1s\n" +
		"http_get:\n" +
		"  interval: 15m\n" +
		"  timeout: 5s\n" +
		"targets: []\n"

	tests := []struct {
		name    string
		icmp    string
		mtr     string
		wantErr string // substring; empty means the config must load cleanly
	}{
		{name: "defaults load", icmp: "", mtr: ""},
		{name: "icmp ttl at lower bound", icmp: "  ttl: 1\n", mtr: ""},
		{name: "icmp ttl at upper bound", icmp: "  ttl: 255\n", mtr: ""},
		// A literal 0 is indistinguishable from "unset" under creasty/defaults, so
		// it is filled with the default (128) and loads cleanly; negatives survive
		// defaults and are rejected.
		{name: "icmp ttl zero is defaulted", icmp: "  ttl: 0\n", mtr: ""},
		{name: "icmp ttl negative rejected", icmp: "  ttl: -1\n", mtr: "", wantErr: "icmp.ttl must be between 1 and 255"},
		{name: "icmp ttl too high rejected", icmp: "  ttl: 256\n", mtr: "", wantErr: "icmp.ttl must be between 1 and 255"},
		{name: "mtr first-ttl at lower bound", icmp: "", mtr: "  first-ttl: 1\n"},
		{name: "mtr first-ttl raised", icmp: "", mtr: "  first-ttl: 5\n"},
		{name: "mtr first-ttl zero is defaulted", icmp: "", mtr: "  first-ttl: 0\n"},
		{name: "mtr first-ttl negative rejected", icmp: "", mtr: "  first-ttl: -1\n", wantErr: "mtr.first-ttl must be between 1 and 255"},
		{name: "mtr first-ttl too high rejected", icmp: "", mtr: "  first-ttl: 256\n", wantErr: "mtr.first-ttl must be between 1 and 255"},
		{name: "first-ttl equal to max-hops rejected", icmp: "", mtr: "  first-ttl: 30\n", wantErr: "mtr.first-ttl must be less than mtr.max-hops"},
		{name: "first-ttl above max-hops rejected", icmp: "", mtr: "  first-ttl: 40\n", wantErr: "mtr.first-ttl must be less than mtr.max-hops"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTempConfig(t, fmt.Sprintf(base, tc.icmp, tc.mtr))

			sc := &SafeConfig{Cfg: &Config{}}
			err := sc.ReloadConfig(logger, path, nil)

			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected clean load, got error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

// TestNormalizeCheckType verifies combined check types are canonicalized so the
// order the operator writes them in does not matter.
func TestNormalizeCheckType(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"ICMP", "ICMP"},
		{"MTR", "MTR"},
		{"TCP", "TCP"},
		{"HTTPGet", "HTTPGet"},
		{"ICMP+MTR", "ICMP+MTR"},
		{"MTR+ICMP", "ICMP+MTR"},
		{"ICMP + MTR", "ICMP+MTR"},   // tolerant of surrounding spaces
		{" MTR + ICMP ", "ICMP+MTR"}, // tolerant of surrounding spaces
		{"Unknown+Type", "Unknown+Type"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := NormalizeCheckType(tc.in); got != tc.want {
			t.Errorf("NormalizeCheckType(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestReloadConfigCombinedTypeOrder is the end-to-end regression for the startup
// failure: a target declared as "MTR+ICMP" (reverse of the canonical order) must
// load cleanly, be normalized to "ICMP+MTR", and not panic in duplicate
// detection. It also confirms two same-named targets with the type written in
// opposite orders are still detected as duplicates.
func TestReloadConfigCombinedTypeOrder(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	const header = "icmp:\n  interval: 3s\n  timeout: 1s\n" +
		"mtr:\n  interval: 3s\n  timeout: 1s\n  max-hops: 30\n" +
		"tcp:\n  interval: 3s\n  timeout: 1s\n" +
		"http_get:\n  interval: 15m\n  timeout: 5s\n"

	t.Run("reverse order loads and normalizes", func(t *testing.T) {
		body := header + "targets:\n" +
			"  - name: t1\n    host: 8.8.8.8\n    type: MTR+ICMP\n"
		path := writeTempConfig(t, body)

		sc := &SafeConfig{Cfg: &Config{}}
		if err := sc.ReloadConfig(logger, path, nil); err != nil {
			t.Fatalf("expected clean load for MTR+ICMP, got: %v", err)
		}
		if len(sc.Cfg.Targets) != 1 {
			t.Fatalf("expected 1 target, got %d", len(sc.Cfg.Targets))
		}
		if got := sc.Cfg.Targets[0].Type; got != "ICMP+MTR" {
			t.Fatalf("expected type normalized to ICMP+MTR, got %q", got)
		}
	})

	t.Run("duplicates detected regardless of order", func(t *testing.T) {
		body := header + "targets:\n" +
			"  - name: dup\n    host: 8.8.8.8\n    type: ICMP+MTR\n" +
			"  - name: dup\n    host: 8.8.4.4\n    type: MTR+ICMP\n"
		path := writeTempConfig(t, body)

		sc := &SafeConfig{Cfg: &Config{}}
		err := sc.ReloadConfig(logger, path, nil)
		if err == nil || !strings.Contains(err.Error(), "found duplicated record") {
			t.Fatalf("expected duplicate detection error, got: %v", err)
		}
	})
}

// TestReloadConfigUnderscoreName is a regression guard confirming that a target
// whose name contains an underscore (e.g. "cloudflare-dns0_1") loads cleanly and
// is preserved verbatim. The name is only ever used as a Prometheus label value
// (underscores are valid there) and as a duplicate-detection map key, so it must
// never trigger a validation error. Two same-typed IP hosts are used so the load
// stays fully offline.
func TestReloadConfigUnderscoreName(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	const header = "icmp:\n  interval: 3s\n  timeout: 1s\n" +
		"mtr:\n  interval: 3s\n  timeout: 1s\n  max-hops: 30\n" +
		"tcp:\n  interval: 3s\n  timeout: 1s\n" +
		"http_get:\n  interval: 15m\n  timeout: 5s\n"

	body := header + "targets:\n" +
		"  - name: cloudflare-dns0_1\n    host: 119.8.16.11\n    type: ICMP+MTR\n" +
		"  - name: a_b_c\n    host: 8.8.4.4\n    type: TCP\n"
	path := writeTempConfig(t, body)

	sc := &SafeConfig{Cfg: &Config{}}
	if err := sc.ReloadConfig(logger, path, nil); err != nil {
		t.Fatalf("expected clean load for underscore names, got: %v", err)
	}
	if len(sc.Cfg.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(sc.Cfg.Targets))
	}

	got := map[string]bool{}
	for _, tgt := range sc.Cfg.Targets {
		got[tgt.Name] = true
	}
	for _, want := range []string{"cloudflare-dns0_1", "a_b_c"} {
		if !got[want] {
			t.Fatalf("expected target name %q to be preserved verbatim, targets: %v", want, got)
		}
	}
}
