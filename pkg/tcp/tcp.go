package tcp

import (
	"fmt"
	"net"
	"time"
)

// Port ICMP Operation
func Port(destAddr string, ip string, srcAddr string, port string, interval time.Duration, timeout time.Duration) (*TCPPortReturn, error) {
	var out TCPPortReturn
	var d net.Dialer
	var err error

	tcpOptions := &TCPPortOptions{}
	tcpOptions.SetInterval(interval)
	tcpOptions.SetTimeout(timeout)

	out.DestAddr = destAddr
	out.DestIp = ip
	out.DestPort = port

	if srcAddr != "" {
		srcIp := net.ParseIP(srcAddr)
		if srcIp == nil {
			out.Success = false
			return &out, fmt.Errorf("source ip: %v is invalid, TCP target: %v", srcAddr, destAddr)
		}
		d = net.Dialer{
			LocalAddr: &net.TCPAddr{
				IP:   srcIp,
				Port: 0,
			},
			Timeout: tcpOptions.Timeout(),
		}
	} else {
		d = net.Dialer{
			Timeout: tcpOptions.Timeout(),
		}
	}

	start := time.Now()
	conn, err := d.Dial("tcp", net.JoinHostPort(ip, port))
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
