package ping

import "time"

const defaultTimeout = 5 * time.Second
const defaultInterval = 10 * time.Millisecond
const defaultPackerSize = 56
const defaultCount = 10
const defaultTTL = 128

// PingResult Calculated results
type PingResult struct {
	Success   bool          `json:"success"`
	DestAddr  string        `json:"dest_address"`
	DropRate  float64       `json:"drop_rate"`
	SumTime   time.Duration `json:"sum"`
	BestTime  time.Duration `json:"best"`
	AvgTime   time.Duration `json:"avg"`
	WorstTime time.Duration `json:"worst"`
	StdDev    time.Duration `json:"stddev"`
}

// PingReturn ICMP Response
type PingReturn struct {
	success   bool
	succSum   int
	allTime   []time.Duration
	sumTime   time.Duration
	bestTime  time.Duration
	avgTime   time.Duration
	worstTime time.Duration
}

// PingOptions ICMP Options
type PingOptions struct {
	count      int
	timeout    time.Duration
	interval   time.Duration
	packetSize int
}

// Count Getter
func (options *PingOptions) Count() int {
	if options.count == 0 {
		options.count = defaultCount
	}
	return options.count
}

// SetCount Setter
func (options *PingOptions) SetCount(count int) {
	options.count = count
}

// Timeout Getter
func (options *PingOptions) Timeout() time.Duration {
	if options.timeout == 0 {
		options.timeout = defaultTimeout
	}
	return options.timeout
}

// SetTimeout Setter
func (options *PingOptions) SetTimeout(timeout time.Duration) {
	options.timeout = timeout
}

// Interval Getter
func (options *PingOptions) Interval() time.Duration {
	if options.interval == 0 {
		options.interval = defaultInterval
	}
	return options.interval
}

// SetInterval Setter
func (options *PingOptions) SetInterval(interval time.Duration) {
	options.interval = interval
}

// PacketSize Getter
func (options *PingOptions) PacketSize() int {
	if options.packetSize == 0 {
		options.packetSize = defaultPackerSize
	}
	return options.packetSize
}

// SetPacketSize Setter
func (options *PingOptions) SetPacketSize(packetSize int) {
	options.packetSize = packetSize
}
