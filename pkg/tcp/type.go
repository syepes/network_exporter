package tcp

import "time"

const defaultTimeout = 5 * time.Second
const defaultInterval = 10 * time.Millisecond

// TCPPortReturn Calculated results
type TCPPortReturn struct {
	Success  bool          `json:"success"`
	DestAddr string        `json:"dest_address"`
	DestPort string        `json:"dest_port"`
	ConTime  time.Duration `json:"connection_time"`
}

// TCPPortOptions ICMP Options
type TCPPortOptions struct {
	timeout  time.Duration
	interval time.Duration
}

// Timeout Getter
func (options *TCPPortOptions) Timeout() time.Duration {
	if options.timeout == 0 {
		options.timeout = defaultTimeout
	}
	return options.timeout
}

// SetTimeout Setter
func (options *TCPPortOptions) SetTimeout(timeout time.Duration) {
	options.timeout = timeout
}

// Interval Getter
func (options *TCPPortOptions) Interval() time.Duration {
	if options.interval == 0 {
		options.interval = defaultInterval
	}
	return options.interval
}

// SetInterval Setter
func (options *TCPPortOptions) SetInterval(interval time.Duration) {
	options.interval = interval
}
