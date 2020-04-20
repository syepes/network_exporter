package ping

import "time"

const defaultTimeout = 5 * time.Second
const defaultInterval = 10 * time.Millisecond
const defaultPackerSize = 56
const defaultCount = 10
const defaultTTL = 128

// PingReturn Calculated results
type PingReturn struct {
	DestAddr string        `json:"dest_address"`
	Success  bool          `json:"success"`
	DropRate float64       `json:"drop_rate"`
	AllTime  time.Duration `json:"all"`
	BestTime time.Duration `json:"best"`
	AvgTime  time.Duration `json:"avg"`
	WrstTime time.Duration `json:"worst"`
	StdDev   float64       `json:"std_dev"`
}

// PingResult ICMP Response
type PingResult struct {
	success  bool
	succSum  int
	allTime  time.Duration
	bestTime time.Duration
	avgTime  time.Duration
	wrstTime time.Duration
}

// PingOptions ICMP Options
type PingOptions struct {
	count      int
	timeoutMs  time.Duration
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
	if options.timeoutMs == 0 {
		options.timeoutMs = defaultTimeout
	}
	return options.timeoutMs
}

// SetTimeout Setter
func (options *PingOptions) SetTimeout(timeoutMs time.Duration) {
	options.timeoutMs = timeoutMs
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
