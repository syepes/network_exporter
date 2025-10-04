package tcp

import (
	"fmt"
	"net"
	"syscall"
	"time"

	"github.com/syepes/network_exporter/pkg/common"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	protocolICMP     = 1  // Internet Control Message
	protocolIPv6ICMP = 58 // ICMP for IPv6
)

// Traceroute performs TCP-based traceroute by sending TCP SYN packets with incrementing TTL
// and listening for ICMP Time Exceeded messages from intermediate routers
func Traceroute(destAddr string, port string, srcAddr string, ttl int, timeout time.Duration, ipv6 bool) (hop common.IcmpReturn, err error) {
	dstIp := net.ParseIP(destAddr)
	if dstIp == nil {
		return hop, fmt.Errorf("destination ip: %v is invalid", destAddr)
	}

	if p4 := dstIp.To4(); len(p4) == net.IPv4len {
		return tcpTracerouteIPv4(destAddr, port, srcAddr, ttl, timeout)
	}
	if ipv6 {
		return tcpTracerouteIPv6(destAddr, port, srcAddr, ttl, timeout)
	}
	return hop, nil
}

func tcpTracerouteIPv4(destAddr string, port string, srcAddr string, ttl int, timeout time.Duration) (hop common.IcmpReturn, err error) {
	hop.Success = false
	start := time.Now()

	// Create ICMP listener to receive Time Exceeded messages
	icmpConn, err := icmp.ListenPacket("ip4:icmp", srcAddr)
	if err != nil {
		return hop, fmt.Errorf("failed to create ICMP listener: %v", err)
	}
	defer icmpConn.Close()

	if err = icmpConn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return hop, err
	}

	// Create TCP connection with custom TTL
	d := &net.Dialer{
		Timeout: timeout,
		Control: func(network, address string, c syscall.RawConn) error {
			var syscallErr error
			err := c.Control(func(fd uintptr) {
				// Set TTL for IPv4 using platform-appropriate type
				syscallErr = setTTLv4(fd, ttl)
			})
			if err != nil {
				return err
			}
			return syscallErr
		},
	}

	if srcAddr != "" {
		srcIp := net.ParseIP(srcAddr)
		if srcIp != nil {
			d.LocalAddr = &net.TCPAddr{IP: srcIp, Port: 0}
		}
	}

	// Start TCP connection attempt (this will send SYN packet with custom TTL)
	connChan := make(chan error, 1)
	go func() {
		conn, err := d.Dial("tcp", net.JoinHostPort(destAddr, port))
		if conn != nil {
			conn.Close()
		}
		connChan <- err
	}()

	// Listen for ICMP Time Exceeded or wait for TCP connection
	for {
		select {
		case connErr := <-connChan:
			// TCP connection completed or failed
			elapsed := time.Since(start)
			if connErr == nil {
				// Successfully connected - we reached the destination
				hop.Elapsed = elapsed
				hop.Addr = destAddr
				hop.Success = true
				return hop, nil
			}
			// Connection failed but we might have gotten ICMP response
			// Continue to check if we received ICMP message
			time.Sleep(10 * time.Millisecond)
			select {
			case <-time.After(timeout - elapsed):
				return hop, fmt.Errorf("timeout waiting for response")
			default:
				// Try to read any pending ICMP message
			}

		case <-time.After(timeout):
			return hop, fmt.Errorf("timeout")
		default:
			// Try to read ICMP message
			b := make([]byte, 1500)
			n, peer, readErr := icmpConn.ReadFrom(b)
			if readErr != nil {
				// No ICMP message yet, continue waiting
				time.Sleep(10 * time.Millisecond)
				continue
			}

			if n > 0 {
				x, err := icmp.ParseMessage(protocolICMP, b[:n])
				if err != nil {
					continue
				}

				// Check for Time Exceeded message
				if x.Type == ipv4.ICMPTypeTimeExceeded {
					elapsed := time.Since(start)
					hop.Elapsed = elapsed
					hop.Addr = peer.String()
					hop.Success = true
					return hop, nil
				}
			}
		}
	}
}

func tcpTracerouteIPv6(destAddr string, port string, srcAddr string, ttl int, timeout time.Duration) (hop common.IcmpReturn, err error) {
	hop.Success = false
	start := time.Now()

	// Create ICMPv6 listener
	icmpConn, err := icmp.ListenPacket("ip6:ipv6-icmp", srcAddr)
	if err != nil {
		return hop, fmt.Errorf("failed to create ICMPv6 listener: %v", err)
	}
	defer icmpConn.Close()

	if err = icmpConn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return hop, err
	}

	// Create TCP connection with custom hop limit (IPv6 equivalent of TTL)
	d := &net.Dialer{
		Timeout: timeout,
		Control: func(network, address string, c syscall.RawConn) error {
			var syscallErr error
			err := c.Control(func(fd uintptr) {
				// Set Hop Limit for IPv6 using platform-appropriate type
				syscallErr = setTTLv6(fd, ttl)
			})
			if err != nil {
				return err
			}
			return syscallErr
		},
	}

	if srcAddr != "" {
		srcIp := net.ParseIP(srcAddr)
		if srcIp != nil {
			d.LocalAddr = &net.TCPAddr{IP: srcIp, Port: 0}
		}
	}

	// Start TCP connection attempt
	connChan := make(chan error, 1)
	go func() {
		conn, err := d.Dial("tcp", net.JoinHostPort(destAddr, port))
		if conn != nil {
			conn.Close()
		}
		connChan <- err
	}()

	// Listen for ICMPv6 Time Exceeded or wait for TCP connection
	for {
		select {
		case connErr := <-connChan:
			elapsed := time.Since(start)
			if connErr == nil {
				// Successfully connected - we reached the destination
				hop.Elapsed = elapsed
				hop.Addr = destAddr
				hop.Success = true
				return hop, nil
			}
			time.Sleep(10 * time.Millisecond)
			select {
			case <-time.After(timeout - elapsed):
				return hop, fmt.Errorf("timeout waiting for response")
			default:
			}

		case <-time.After(timeout):
			return hop, fmt.Errorf("timeout")
		default:
			b := make([]byte, 1500)
			n, peer, readErr := icmpConn.ReadFrom(b)
			if readErr != nil {
				time.Sleep(10 * time.Millisecond)
				continue
			}

			if n > 0 {
				x, err := icmp.ParseMessage(protocolIPv6ICMP, b[:n])
				if err != nil {
					continue
				}

				// Check for Time Exceeded message
				if x.Type == ipv6.ICMPTypeTimeExceeded {
					elapsed := time.Since(start)
					hop.Elapsed = elapsed
					hop.Addr = peer.String()
					hop.Success = true
					return hop, nil
				}
			}
		}
	}
}
