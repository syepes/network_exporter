package common

import (
	"time"
)

// IcmpReturn ICMP Response time details
type IcmpReturn struct {
	Success bool
	Addr    string
	Elapsed time.Duration
}

// IcmpHop ICMP HOP Response time details
type IcmpHop struct {
	Success     bool          `json:"success"`
	AddressFrom string        `json:"address_from"`
	AddressTo   string        `json:"address_to"`
	N           int           `json:"n"`
	TTL         int           `json:"ttl"`
	Snt         int           `json:"snt"`
	StdDev      time.Duration `json:"stddev"`
	LastTime    time.Duration `json:"last"`
	SumTime     time.Duration `json:"sum"`
	AvgTime     time.Duration `json:"avg"`
	BestTime    time.Duration `json:"best"`
	WorstTime   time.Duration `json:"worst"`
	Loss        float32       `json:"loss"`
}
