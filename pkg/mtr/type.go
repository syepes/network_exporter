package mtr

import (
	"time"

	"github.com/syepes/ping_exporter/pkg/common"
)

const defaultMaxHops = 30
const defaultTimeout = 5 * time.Second
const defaultPackerSize = 56
const defaultSntSize = 10

type MtrReturn struct {
	Success  bool
	TTL      int
	Host     string
	SuccSum  int
	LastTime time.Duration
	AllTime  time.Duration
	BestTime time.Duration
	AvgTime  time.Duration
	WrstTime time.Duration
}

type MtrResult struct {
	DestAddress string           `json:"dest_address"`
	Hops        []common.IcmpHop `json:"hops"`
}

type MtrOptions struct {
	maxHops    int
	timeout    time.Duration
	packetSize int
	sntSize    int
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

// SntSize Getter
func (options *MtrOptions) SntSize() int {
	if options.sntSize == 0 {
		options.sntSize = defaultSntSize
	}
	return options.sntSize
}

// SetSntSize Setter
func (options *MtrOptions) SetSntSize(sntSize int) {
	options.sntSize = sntSize
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
