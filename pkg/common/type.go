package common

import (
	"sync/atomic"
	"time"
)

// IcmpID ICMP Echo Unique ID for each coroutine.
//
// SCALING LIMITS:
// The ICMP ID counter is shared across all PING and MTR operations and cycles from 1-65500.
// This limits the total concurrent ICMP operations to approximately 65,500 across all targets.
// With default settings (3 concurrent jobs per target), this supports:
//   - ~20,000 PING targets (assuming 3 operations each)
//   - ~1,000 MTR targets (MTR uses more ICMP IDs per operation)
// The counter automatically resets when reaching 65500 to prevent exhaustion.
type IcmpID struct {
	icmpID int32
}

// Get ICMP Echo Unique ID.
// Returns a unique ICMP ID for the current operation, automatically cycling
// from 1 to 65500 to prevent ID exhaustion in high-scale deployments.
func (c *IcmpID) Get() int32 {
	for {
		val := atomic.LoadInt32(&c.icmpID)
		// Init
		if val == 0 {
			atomic.StoreInt32(&c.icmpID, 1)
			val = 1
		}
		// Reset Counter
		if atomic.CompareAndSwapInt32(&c.icmpID, 65500, 2) {
			return 1
		}
		if atomic.CompareAndSwapInt32(&c.icmpID, val, val+1) {
			return val
		}
	}
}

// IcmpReturn ICMP Response time details
type IcmpReturn struct {
	Success bool
	Addr    string
	Elapsed time.Duration
}

// IcmpSummary ICMP HOP Summary
type IcmpSummary struct {
	AddressFrom string        `json:"address_from"`
	AddressTo   string        `json:"address_to"`
	Snt         int           `json:"snt"`
	SntFail     int           `json:"snt_fail"`
	SntTime     time.Duration `json:"snt_time"`
}

// IcmpHop ICMP HOP Response time details
type IcmpHop struct {
	Success              bool          `json:"success"`
	AddressFrom          string        `json:"address_from"`
	AddressTo            string        `json:"address_to"`
	N                    int           `json:"n"`
	TTL                  int           `json:"ttl"`
	Snt                  int           `json:"snt"`
	SntFail              int           `json:"snt_fail"`
	LastTime             time.Duration `json:"last"`
	SumTime              time.Duration `json:"sum"`
	AvgTime              time.Duration `json:"avg"`
	BestTime             time.Duration `json:"best"`
	WorstTime            time.Duration `json:"worst"`
	SquaredDeviationTime time.Duration `json:"sd"`
	UncorrectedSDTime    time.Duration `json:"usd"`
	CorrectedSDTime      time.Duration `json:"csd"`
	RangeTime            time.Duration `json:"range"`
	Loss                 float64       `json:"loss"`
}
