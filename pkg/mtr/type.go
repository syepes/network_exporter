package mtr

import (
	"time"

	"github.com/syepes/network_exporter/pkg/common"
)

const defaultMaxHops = 30
const defaultTimeout = 5 * time.Second
const defaultPackerSize = 56
const defaultCount = 10

// MtrResult Calculated results
type MtrResult struct {
	DestAddr string           `json:"dest_address"`
	Hops     []common.IcmpHop `json:"hops"`
}

// MtrReturn MTR Response
// TODO: Add Jitter
// RFC 1889 (http://www.ietf.org/rfc/rfc1889.txt) is the RFC for RTP, later superseded by RFC 3550.
// How interarrival jitter is calculated RFC 3550 (http://www.ietf.org/rfc/rfc3550.txt)
type MtrReturn struct {
	success   bool
	ttl       int
	host      string
	succSum   int
	lastTime  time.Duration
	allTime   []time.Duration
	sumTime   time.Duration
	bestTime  time.Duration
	avgTime   time.Duration
	worstTime time.Duration
}

// MtrOptions MTR Options
type MtrOptions struct {
	maxHops    int
	timeout    time.Duration
	packetSize int
	count      int
}

// MaxHops Getter
func (options *MtrOptions) MaxHops() int {
	if options.maxHops == 0 {
		options.maxHops = defaultMaxHops
	}
	return options.maxHops
}

// SetMaxHops Setter
func (options *MtrOptions) SetMaxHops(maxHops int) {
	options.maxHops = maxHops
}

// Timeout Getter
func (options *MtrOptions) Timeout() time.Duration {
	if options.timeout == 0 {
		options.timeout = defaultTimeout
	}
	return options.timeout
}

// SetTimeout Setter
func (options *MtrOptions) SetTimeout(timeout time.Duration) {
	options.timeout = timeout
}

// Count Getter
func (options *MtrOptions) Count() int {
	if options.count == 0 {
		options.count = defaultCount
	}
	return options.count
}

// SetCount Setter
func (options *MtrOptions) SetCount(count int) {
	options.count = count
}

// PacketSize Getter
func (options *MtrOptions) PacketSize() int {
	if options.packetSize == 0 {
		options.packetSize = defaultPackerSize
	}
	return options.packetSize
}

// SetPacketSize Setter
func (options *MtrOptions) SetPacketSize(packetSize int) {
	options.packetSize = packetSize
}
