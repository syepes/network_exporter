package common

import (
	"sync/atomic"
	"time"
)

// IcmpID ICMP Echo Unique ID for each coroutine
type IcmpID struct {
	icmpID int32
}

// Get ICMP Echo Unique ID
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

// IcmpHop ICMP HOP Response time details
type IcmpHop struct {
	Success              bool          `json:"success"`
	AddressFrom          string        `json:"address_from"`
	AddressTo            string        `json:"address_to"`
	N                    int           `json:"n"`
	TTL                  int           `json:"ttl"`
	Snt                  int           `json:"snt"`
	LastTime             time.Duration `json:"last"`
	SumTime              time.Duration `json:"sum"`
	AvgTime              time.Duration `json:"avg"`
	BestTime             time.Duration `json:"best"`
	WorstTime            time.Duration `json:"worst"`
	SquaredDeviationTime time.Duration `json:"sd"`
	UncorrectedSDTime    time.Duration `json:"usd"`
	CorrectedSDTime      time.Duration `json:"csd"`
	RangeTime            time.Duration `json:"range"`
	Loss                 float32       `json:"loss"`
}
