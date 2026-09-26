package common

import (
	"testing"
	"time"
)

// TestComputeTimeStatsMatchesIndividualFunctions guards the single-pass
// ComputeTimeStats refactor: it must produce exactly the same values as the
// individual TimeUncorrectedDeviation / TimeCorrectedDeviation / TimeRange
// functions it replaces at the call sites, across empty, single and multi-sample
// inputs.
func TestComputeTimeStatsMatchesIndividualFunctions(t *testing.T) {
	cases := map[string][]time.Duration{
		"empty":     {},
		"single":    {5 * time.Millisecond},
		"identical": {7 * time.Millisecond, 7 * time.Millisecond, 7 * time.Millisecond},
		"spread":    {1 * time.Millisecond, 4 * time.Millisecond, 2 * time.Millisecond, 9 * time.Millisecond, 3 * time.Millisecond},
		"unordered": {900 * time.Microsecond, 100 * time.Microsecond, 500 * time.Microsecond},
	}

	for name, values := range cases {
		t.Run(name, func(t *testing.T) {
			got := ComputeTimeStats(values)

			wantUSD := time.Duration(TimeUncorrectedDeviation(values))
			wantCSD := time.Duration(TimeCorrectedDeviation(values))
			wantRange := time.Duration(TimeRange(values))

			if got.UncorrectedSD != wantUSD {
				t.Errorf("UncorrectedSD = %v, want %v", got.UncorrectedSD, wantUSD)
			}
			if got.CorrectedSD != wantCSD {
				t.Errorf("CorrectedSD = %v, want %v", got.CorrectedSD, wantCSD)
			}
			if got.Range != wantRange {
				t.Errorf("Range = %v, want %v", got.Range, wantRange)
			}
		})
	}
}
