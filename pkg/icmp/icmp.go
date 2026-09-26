package icmp

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/syepes/network_exporter/pkg/common"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// https://hechao.li/2018/09/27/How-Is-Ping-Deduplexed/
const (
	protocolICMP     = 1  // Internet Control Message
	protocolIPv6ICMP = 58 // ICMP for IPv6
)

// Icmp Validate IP and check the version
func Icmp(destAddr string, srcAddr string, ttl int, pid int, timeout time.Duration, seq int, payloadSize int, ipv6 bool) (hop common.IcmpReturn, err error) {
	dstIp := net.ParseIP(destAddr)
	if dstIp == nil {
		return hop, fmt.Errorf("destination ip: %v is invalid", destAddr)
	}

	ipAddr := net.IPAddr{IP: dstIp}

	if srcAddr != "" {
		srcIp := net.ParseIP(srcAddr)
		if srcIp == nil {
			return hop, fmt.Errorf("source ip: %v is invalid, target: %v", srcAddr, destAddr)
		}

		if p4 := dstIp.To4(); len(p4) == net.IPv4len {
			return icmpIpv4(srcAddr, &ipAddr, ttl, pid, timeout, seq, payloadSize)
		}
		if ipv6 {
			return icmpIpv6(srcAddr, &ipAddr, ttl, pid, timeout, seq, payloadSize)
		}
		// IPv6 destination but IPv6 is disabled: return an unsuccessful hop
		// (Success=false) with no error. In practice DestAddrs already filters
		// IPv6 addresses when IPv6 is disabled, so this branch is rarely reached.
		return hop, nil
	}

	if p4 := dstIp.To4(); len(p4) == net.IPv4len {
		return icmpIpv4("0.0.0.0", &ipAddr, ttl, pid, timeout, seq, payloadSize)
	}
	if ipv6 {
		return icmpIpv6("::", &ipAddr, ttl, pid, timeout, seq, payloadSize)
	}
	// IPv6 destination but IPv6 is disabled: return an unsuccessful hop
	// (Success=false) with no error. See the note above.
	return hop, nil
}

func icmpIpv4(localAddr string, dst net.Addr, ttl int, pid int, timeout time.Duration, seq int, payloadSize int) (hop common.IcmpReturn, err error) {
	hop.Success = false
	start := time.Now()
	c, err := icmp.ListenPacket("ip4:icmp", localAddr)
	if err != nil {
		return hop, err
	}
	defer c.Close()

	if err = c.IPv4PacketConn().SetTTL(ttl); err != nil {
		return hop, err
	}

	if err = c.SetDeadline(time.Now().Add(timeout)); err != nil {
		return hop, err
	}

	// Create payload: 4-byte sequence number + (payloadSize - 4) filler bytes
	bs := make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, uint32(seq))

	// Generate dynamic payload with specified size
	payload := make([]byte, payloadSize)
	copy(payload, bs) // First 4 bytes are sequence number
	for i := 4; i < payloadSize; i++ {
		payload[i] = 'x' // Fill remaining bytes
	}

	wm := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   pid,
			Seq:  seq,
			Data: payload,
		},
	}

	wb, err := wm.Marshal(nil)
	if err != nil {
		return hop, err
	}

	if _, err := c.WriteTo(wb, dst); err != nil {
		return hop, err
	}

	peer, _, err := listenForSpecific4(c, payload, pid, seq, wb)
	if err != nil {
		return hop, err
	}

	elapsed := time.Since(start)
	hop.Elapsed = elapsed
	hop.Addr = peer
	hop.Success = true
	return hop, err
}

func icmpIpv6(localAddr string, dst net.Addr, ttl, pid int, timeout time.Duration, seq int, payloadSize int) (hop common.IcmpReturn, err error) {
	hop.Success = false
	start := time.Now()
	c, err := icmp.ListenPacket("ip6:ipv6-icmp", localAddr)
	if err != nil {
		return hop, err
	}
	defer c.Close()

	if err = c.IPv6PacketConn().SetHopLimit(ttl); err != nil {
		return hop, err
	}

	if err = c.SetDeadline(time.Now().Add(timeout)); err != nil {
		return hop, err
	}

	// Create payload: 4-byte sequence number + (payloadSize - 4) filler bytes
	bs := make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, uint32(seq))

	// Generate dynamic payload with specified size
	payload := make([]byte, payloadSize)
	copy(payload, bs) // First 4 bytes are sequence number
	for i := 4; i < payloadSize; i++ {
		payload[i] = 'x' // Fill remaining bytes
	}

	wm := icmp.Message{
		Type: ipv6.ICMPTypeEchoRequest,
		Code: 0,
		Body: &icmp.Echo{
			ID:   pid,
			Seq:  seq,
			Data: payload,
		},
	}
	wb, err := wm.Marshal(nil)
	if err != nil {
		return hop, err
	}

	if _, err := c.WriteTo(wb, dst); err != nil {
		return hop, err
	}

	peer, _, err := listenForSpecific6(c, payload, pid, seq)
	if err != nil {
		return hop, err
	}

	elapsed := time.Since(start)
	hop.Elapsed = elapsed
	hop.Addr = peer
	hop.Success = true
	return hop, err
}

// Listen IPv4 icmp returned packet and verify the content
func listenForSpecific4(conn *icmp.PacketConn, neededBody []byte, needID int, needSeq int, sent []byte) (string, []byte, error) {
	for {
		b := make([]byte, 1500)
		n, peer, err := conn.ReadFrom(b)
		// Any read error (including the read deadline / timeout that bounds this
		// loop) means no matching reply arrived; return it so the caller counts a
		// drop. Returning on every error also avoids spinning on a persistent,
		// non-temporary error.
		if err != nil {
			return "", []byte{}, err
		}
		if n == 0 {
			continue
		}

		x, err := icmp.ParseMessage(protocolICMP, b[:n])
		if err != nil {
			continue
		}

		// A received packet may be malformed or truncated; guard every type
		// assertion and slice so a crafted reply cannot panic this probe
		// goroutine (there is no recover here).
		msgType, ok := x.Type.(ipv4.ICMPType)
		if !ok {
			continue
		}

		switch msgType {
		case ipv4.ICMPTypeTimeExceeded:
			te, ok := x.Body.(*icmp.TimeExceeded)
			if !ok {
				continue
			}
			body := te.Data
			oh, err := ipv4.ParseHeader(body)
			if err != nil || len(body) < oh.Len {
				continue
			}
			inner, err := icmp.ParseMessage(protocolICMP, body[oh.Len:])
			if err != nil {
				continue
			}
			if echo, ok := inner.Body.(*icmp.Echo); ok {
				if echo.ID == needID && echo.Seq == needSeq {
					return peer.String(), []byte{}, nil
				}
			}
		case ipv4.ICMPTypeEchoReply:
			echo, ok := x.Body.(*icmp.Echo)
			if !ok {
				continue
			}
			b, err := x.Body.Marshal(protocolICMP)
			if err != nil || len(b) < 4 {
				continue
			}
			if string(b[4:]) != string(neededBody) || echo.ID != needID {
				continue
			}
			return peer.String(), b[4:], nil
		}
	}
}

// Listen IPv6 icmp returned packet and verify the content
func listenForSpecific6(conn *icmp.PacketConn, neededBody []byte, needID int, needSeq int) (string, []byte, error) {
	for {
		b := make([]byte, 1500)
		n, peer, err := conn.ReadFrom(b)
		// Any read error (including the read deadline / timeout that bounds this
		// loop) means no matching reply arrived; return it so the caller counts a
		// drop. Returning on every error also avoids spinning on a persistent,
		// non-temporary error.
		if err != nil {
			return "", []byte{}, err
		}
		if n == 0 {
			continue
		}

		x, err := icmp.ParseMessage(protocolIPv6ICMP, b[:n])
		if err != nil {
			continue
		}

		// A received packet may be malformed or truncated; guard every type
		// assertion and slice so a crafted reply cannot panic this probe
		// goroutine (there is no recover here).
		msgType, ok := x.Type.(ipv6.ICMPType)
		if !ok {
			continue
		}

		switch msgType {
		case ipv6.ICMPTypeTimeExceeded:
			te, ok := x.Body.(*icmp.TimeExceeded)
			if !ok {
				continue
			}
			body := te.Data
			// The quoted packet is a fixed 40-byte IPv6 header (no extension
			// headers for these probes) followed by the original datagram.
			if len(body) < ipv6.HeaderLen {
				continue
			}
			inner, err := icmp.ParseMessage(protocolIPv6ICMP, body[ipv6.HeaderLen:])
			if err != nil {
				continue
			}
			if echo, ok := inner.Body.(*icmp.Echo); ok {
				if echo.ID == needID && echo.Seq == needSeq {
					return peer.String(), []byte{}, nil
				}
			}
		case ipv6.ICMPTypeEchoReply:
			echo, ok := x.Body.(*icmp.Echo)
			if !ok {
				continue
			}
			b, err := x.Body.Marshal(protocolIPv6ICMP)
			if err != nil || len(b) < 4 {
				continue
			}
			if string(b[4:]) != string(neededBody) || echo.ID != needID {
				continue
			}
			return peer.String(), b[4:], nil
		}
	}
}
