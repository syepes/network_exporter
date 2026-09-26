package tcp

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
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
	// IPv6 destination but IPv6 is disabled: return an unsuccessful hop
	// (Success=false) with no error. In practice the caller resolves targets
	// with IPv6 filtered out when IPv6 is disabled, so this is rarely reached.
	return hop, nil
}

func tcpTracerouteIPv4(destAddr string, port string, srcAddr string, ttl int, timeout time.Duration) (hop common.IcmpReturn, err error) {
	hop.Success = false
	start := time.Now()
	deadline := start.Add(timeout)

	// Flow identity used to confirm that an ICMP Time Exceeded quotes THIS
	// probe's packet rather than another concurrent probe's.
	wantDst := net.ParseIP(destAddr)
	wantSrc := net.ParseIP(srcAddr) // nil when srcAddr is empty
	wantDstPort, _ := strconv.Atoi(port)

	// Create ICMP listener to receive Time Exceeded messages
	icmpConn, err := icmp.ListenPacket("ip4:icmp", srcAddr)
	if err != nil {
		return hop, fmt.Errorf("failed to create ICMP listener: %v", err)
	}
	defer icmpConn.Close()

	if err = icmpConn.SetReadDeadline(deadline); err != nil {
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

	// Start TCP connection attempt (this sends the SYN with the custom TTL)
	connChan := make(chan error, 1)
	go func() {
		conn, err := d.Dial("tcp", net.JoinHostPort(destAddr, port))
		if conn != nil {
			conn.Close()
		}
		connChan <- err
	}()

	// Read matching ICMP Time Exceeded messages in the background. The reader
	// only reports a hit that quotes this probe's flow and exits on the read
	// deadline or when the connection is closed (via the deferred Close).
	icmpChan := make(chan string, 1)
	go func() {
		for {
			b := make([]byte, 1500)
			n, peer, readErr := icmpConn.ReadFrom(b)
			if readErr != nil {
				return
			}
			if n == 0 {
				continue
			}
			msg, err := icmp.ParseMessage(protocolICMP, b[:n])
			if err != nil {
				continue
			}
			if msg.Type != ipv4.ICMPTypeTimeExceeded {
				continue
			}
			te, ok := msg.Body.(*icmp.TimeExceeded)
			if !ok {
				continue
			}
			if !icmpMatchesFlowV4(te.Data, wantSrc, wantDst, wantDstPort) {
				continue
			}
			icmpChan <- peer.String()
			return
		}
	}()

	// Whichever completes first wins: a finished TCP dial (we reached the
	// destination), a flow-matched Time Exceeded (an intermediate hop), or the
	// overall timeout.
	select {
	case connErr := <-connChan:
		if connErr == nil {
			hop.Elapsed = time.Since(start)
			hop.Addr = destAddr
			hop.Success = true
			return hop, nil
		}
		// The dial failed (SYN dropped at an intermediate hop, or refused/timed
		// out). Give the ICMP reader the remaining budget to surface a matching
		// Time Exceeded before giving up.
		select {
		case peer := <-icmpChan:
			hop.Elapsed = time.Since(start)
			hop.Addr = peer
			hop.Success = true
			return hop, nil
		case <-time.After(time.Until(deadline)):
			return hop, fmt.Errorf("timeout")
		}
	case peer := <-icmpChan:
		hop.Elapsed = time.Since(start)
		hop.Addr = peer
		hop.Success = true
		return hop, nil
	case <-time.After(time.Until(deadline)):
		return hop, fmt.Errorf("timeout")
	}
}

func tcpTracerouteIPv6(destAddr string, port string, srcAddr string, ttl int, timeout time.Duration) (hop common.IcmpReturn, err error) {
	hop.Success = false
	start := time.Now()
	deadline := start.Add(timeout)

	wantDst := net.ParseIP(destAddr)
	wantSrc := net.ParseIP(srcAddr) // nil when srcAddr is empty
	wantDstPort, _ := strconv.Atoi(port)

	// Create ICMPv6 listener
	icmpConn, err := icmp.ListenPacket("ip6:ipv6-icmp", srcAddr)
	if err != nil {
		return hop, fmt.Errorf("failed to create ICMPv6 listener: %v", err)
	}
	defer icmpConn.Close()

	if err = icmpConn.SetReadDeadline(deadline); err != nil {
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

	icmpChan := make(chan string, 1)
	go func() {
		for {
			b := make([]byte, 1500)
			n, peer, readErr := icmpConn.ReadFrom(b)
			if readErr != nil {
				return
			}
			if n == 0 {
				continue
			}
			msg, err := icmp.ParseMessage(protocolIPv6ICMP, b[:n])
			if err != nil {
				continue
			}
			if msg.Type != ipv6.ICMPTypeTimeExceeded {
				continue
			}
			te, ok := msg.Body.(*icmp.TimeExceeded)
			if !ok {
				continue
			}
			if !icmpMatchesFlowV6(te.Data, wantSrc, wantDst, wantDstPort) {
				continue
			}
			icmpChan <- peer.String()
			return
		}
	}()

	select {
	case connErr := <-connChan:
		if connErr == nil {
			hop.Elapsed = time.Since(start)
			hop.Addr = destAddr
			hop.Success = true
			return hop, nil
		}
		select {
		case peer := <-icmpChan:
			hop.Elapsed = time.Since(start)
			hop.Addr = peer
			hop.Success = true
			return hop, nil
		case <-time.After(time.Until(deadline)):
			return hop, fmt.Errorf("timeout")
		}
	case peer := <-icmpChan:
		hop.Elapsed = time.Since(start)
		hop.Addr = peer
		hop.Success = true
		return hop, nil
	case <-time.After(time.Until(deadline)):
		return hop, fmt.Errorf("timeout")
	}
}

// icmpMatchesFlowV4 reports whether the payload quoted inside an ICMPv4 Time
// Exceeded message (the original IPv4 header plus the first bytes of the
// original datagram) belongs to the probe identified by wantSrc/wantDst and the
// TCP destination port wantDstPort. The source (ephemeral) port is chosen by
// the kernel and therefore is not matched.
func icmpMatchesFlowV4(data []byte, wantSrc, wantDst net.IP, wantDstPort int) bool {
	oh, err := ipv4.ParseHeader(data)
	if err != nil {
		return false
	}
	if wantDst != nil && !oh.Dst.Equal(wantDst) {
		return false
	}
	if wantSrc != nil && !oh.Src.Equal(wantSrc) {
		return false
	}
	// The quoted transport header follows the IPv4 header. RFC 792 guarantees at
	// least the first 8 bytes of the original datagram, which covers the TCP
	// source (2) and destination (2) ports.
	if wantDstPort != 0 {
		if len(data) < oh.Len+4 {
			return false
		}
		dstPort := int(binary.BigEndian.Uint16(data[oh.Len+2 : oh.Len+4]))
		if dstPort != wantDstPort {
			return false
		}
	}
	return true
}

// icmpMatchesFlowV6 reports whether the payload quoted inside an ICMPv6 Time
// Exceeded message belongs to the probe identified by wantSrc/wantDst and the
// TCP destination port wantDstPort. It assumes the quoted packet has no IPv6
// extension headers (true for the TCP SYN probes emitted here), consistent with
// the fixed-offset parsing used elsewhere in this codebase.
func icmpMatchesFlowV6(data []byte, wantSrc, wantDst net.IP, wantDstPort int) bool {
	h, err := ipv6.ParseHeader(data)
	if err != nil {
		return false
	}
	if wantDst != nil && !h.Dst.Equal(wantDst) {
		return false
	}
	if wantSrc != nil && !h.Src.Equal(wantSrc) {
		return false
	}
	if wantDstPort != 0 {
		if len(data) < ipv6.HeaderLen+4 {
			return false
		}
		dstPort := int(binary.BigEndian.Uint16(data[ipv6.HeaderLen+2 : ipv6.HeaderLen+4]))
		if dstPort != wantDstPort {
			return false
		}
	}
	return true
}
