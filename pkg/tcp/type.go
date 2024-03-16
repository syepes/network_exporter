package tcp

import "time"

const defaultTimeout = 5 * time.Second

// TCPPortReturn Calculated results
type TCPPortReturn struct {
	Success  bool          `json:"success"`
	DestAddr string        `json:"dest_address"`
	DestIp   string        `json:"dest_ip"`
	DestPort string        `json:"dest_port"`
	SrcIp    string        `json:"src_ip"`
	ConTime  time.Duration `json:"connection_time"`
}

// TCPPortOptions ICMP Options
type TCPPortOptions struct {
	timeout  time.Duration
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
