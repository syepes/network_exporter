package http

import (
	"sync"
	"time"
)

// HTTPReturn Calculated results
type HTTPReturn struct {
	Success               bool          `json:"success"`
	DestAddr              string        `json:"dest_address"`
	Status                int           `json:"status,omitempty"`
	ContentLength         int64         `json:"content_length,omitempty"`
	DNSLookup             time.Duration `json:"dnsLookup,omitempty"`
	TCPConnection         time.Duration `json:"tcpConnection,omitempty"`
	TLSHandshake          time.Duration `json:"tlsHandshake,omitempty"`
	TLSVersion            string        `json:"tlsVersion,omitempty"`
	TLSEarliestCertExpiry time.Time     `json:"tlsEarliestCertExpiry,omitempty"`
	TLSLastChainExpiry    time.Time     `json:"tlsLastChainExpiry,omitempty"`
	ServerProcessing      time.Duration `json:"serverProcessing,omitempty"`
	ContentTransfer       time.Duration `json:"contentTransfer,omitempty"`
	Total                 time.Duration `json:"total,omitempty"`
}

// HTTPTimelineStats http timeline stats
type HTTPTimelineStats struct {
	DNSLookup        time.Duration `json:"dnsLookup,omitempty"`
	TCPConnection    time.Duration `json:"tcpConnection,omitempty"`
	TLSHandshake     time.Duration `json:"tlsHandshake,omitempty"`
	ServerProcessing time.Duration `json:"serverProcessing,omitempty"`
	ContentTransfer  time.Duration `json:"contentTransfer,omitempty"`
	Total            time.Duration `json:"total,omitempty"`
}

// HTTPTrace http trace
type HTTPTrace struct {
	Host                 string        `json:"host,omitempty"`
	Addrs                []string      `json:"addrs,omitempty"`
	Network              string        `json:"network,omitempty"`
	Addr                 string        `json:"addr,omitempty"`
	Reused               bool          `json:"reused,omitempty"`
	TCPReused            bool          `json:"tcpReused,omitempty"`
	WasIdle              bool          `json:"wasIdle,omitempty"`
	IdleTime             time.Duration `json:"idleTime,omitempty"`
	Protocol             string        `json:"protocol,omitempty"`
	TLSResume            bool          `json:"tlsResume,omitempty"`
	Start                time.Time     `json:"start,omitempty"`
	DNSStart             time.Time     `json:"dnsStart,omitempty"`
	DNSDone              time.Time     `json:"dnsDone,omitempty"`
	ConnectStart         time.Time     `json:"connectStart,omitempty"`
	ConnectDone          time.Time     `json:"connectDone,omitempty"`
	GotConnect           time.Time     `json:"gotConnect,omitempty"`
	GotFirstResponseByte time.Time     `json:"gotFirstResponseByte,omitempty"`
	TLSHandshakeStart    time.Time     `json:"tlsHandshakeStart,omitempty"`
	TLSHandshakeDone     time.Time     `json:"tlsHandshakeDone,omitempty"`
	Done                 time.Time     `json:"done,omitempty"`
	sync.RWMutex                       // Because the timeout setting may cause trace to read and write coexist, we need the lock
}
