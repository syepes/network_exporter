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
		return nil, fmt.Errorf("TCP Port Failed due to an error: %v", err)
	}

	out.DestAddr = addr
	out.DestPort = port

	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(addr, port), tcpOptions.Timeout())
	out.ConTime = time.Since(start)

	if err != nil {
		out.Success = false
	} else {
		defer conn.Close()

		// Set Deadline timeout
		if err := conn.SetDeadline(time.Now().Add(tcpOptions.Timeout())); err != nil {
			out.Success = false
			return &out, fmt.Errorf("Error setting deadline timout", "err", err)
		}

		if conn != nil {
			out.Success = true
		} else {
			out.Success = false
		}
	}

	return &out, nil
}
