package tcp

import (
	"fmt"
	"net"
	"time"

	"github.com/syepes/ping_exporter/pkg/common"
)

// Port ICMP Operation
func Port(addr string, port string, interval time.Duration, timeout time.Duration) (*TCPPortReturn, error) {
	var out TCPPortReturn
	var err error

	tcpOptions := &TCPPortOptions{}
	tcpOptions.SetInterval(interval)
	tcpOptions.SetTimeout(timeout)

	// Resolve hostnames
	ipAddrs, err := common.DestAddrs(addr)
	if err != nil || len(ipAddrs) == 0 {
		return nil, fmt.Errorf("Ping Failed due to an error: %v", err)
	}

	out.DestAddr = addr
	out.DestPort = port

	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(addr, port), tcpOptions.Timeout())
	elapsed := time.Since(start)
	if err != nil {
		out.Success = false
		out.ConTime = elapsed
	}
	if conn != nil {
		defer conn.Close()
		out.Success = true
		out.ConTime = elapsed
	}

	return &out, nil
}
