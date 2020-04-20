package icmp

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/syepes/ping_exporter/pkg/common"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	protocolICMP     = 1  // Internet Control Message
	protocolIPv6ICMP = 58 // ICMP for IPv6
)

// Icmp Validate IP and check the version
func Icmp(destAddr string, ttl, pid int, timeout time.Duration, seq int) (hop common.IcmpReturn, err error) {
	ip := net.ParseIP(destAddr)
	if ip == nil {
		return hop, fmt.Errorf("ip: %v is invalid", destAddr)
	}
	ipAddr := net.IPAddr{IP: ip}

	if p4 := ip.To4(); len(p4) == net.IPv4len {
		return icmpIpv4("0.0.0.0", &ipAddr, ttl, pid, timeout, seq)
	}
	return icmpIpv6("::", &ipAddr, ttl, pid, timeout, seq)
}

func icmpIpv4(localAddr string, dst net.Addr, ttl, pid int, timeout time.Duration, seq int) (hop common.IcmpReturn, err error) {
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

	bs := make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, uint32(seq))
	wm := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID: pid, Seq: seq,
			Data: append(bs, 'x'),
		},
	}

	wb, err := wm.Marshal(nil)
	if err != nil {
		return hop, err
	}

	if _, err := c.WriteTo(wb, dst); err != nil {
		return hop, err
	}

	peer, _, err := listenForSpecific4(c, append(bs, 'x'), pid, seq)
	if err != nil {
		return hop, err
	}

	elapsed := time.Since(start)
	hop.Elapsed = elapsed
	hop.Addr = peer
	hop.Success = true
	return hop, err
}

func icmpIpv6(localAddr string, dst net.Addr, ttl, pid int, timeout time.Duration, seq int) (hop common.IcmpReturn, err error) {
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

	bs := make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, uint32(seq))
	wm := icmp.Message{
		Type: ipv6.ICMPTypeEchoRequest,
		Code: 0,
		Body: &icmp.Echo{
			ID: pid, Seq: seq,
			Data: append(bs, 'x'),
		},
	}
	wb, err := wm.Marshal(nil)
	if err != nil {
		return hop, err
	}

	if _, err := c.WriteTo(wb, dst); err != nil {
		return hop, err
	}

	peer, _, err := listenForSpecific6(c, append(bs, 'x'), pid, seq)
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
func listenForSpecific4(conn *icmp.PacketConn, neededBody []byte, needID int, needSeq int) (string, []byte, error) {
	for {
		b := make([]byte, 1500)
		n, peer, err := conn.ReadFrom(b)
		if err != nil {
			if neterr, ok := err.(*net.OpError); ok {
				return "", []byte{}, neterr
			}
		}
		if n == 0 {
			continue
		}

		x, err := icmp.ParseMessage(protocolICMP, b[:n])
		if err != nil {
			continue
		}

		if typ, ok := x.Type.(ipv4.ICMPType); ok && typ.String() == "time exceeded" {
			body := x.Body.(*icmp.TimeExceeded).Data

			x, _ := icmp.ParseMessage(protocolICMP, body[20:])
			switch x.Body.(type) {
			case *icmp.Echo:
				msg := x.Body.(*icmp.Echo)
				if msg.ID == needID && msg.Seq == needSeq {
					return peer.String(), []byte{}, nil
				}
			default:
				// ignore
			}
		}

		if typ, ok := x.Type.(ipv4.ICMPType); ok && typ.String() == "echo reply" {
			b, _ := x.Body.Marshal(protocolICMP)
			if string(b[4:]) != string(neededBody) || x.Body.(*icmp.Echo).ID != needID {
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
		if err != nil {
			if neterr, ok := err.(*net.OpError); ok {
				return "", []byte{}, neterr
			}
		}
		if n == 0 {
			continue
		}

		x, err := icmp.ParseMessage(protocolIPv6ICMP, b[:n])
		if err != nil {
			continue
		}

		if x.Type.(ipv6.ICMPType) == ipv6.ICMPTypeTimeExceeded {
			body := x.Body.(*icmp.TimeExceeded).Data
			x, _ := icmp.ParseMessage(protocolIPv6ICMP, body[40:])
			switch x.Body.(type) {
			case *icmp.Echo:
				msg := x.Body.(*icmp.Echo)
				if msg.ID == needID && msg.Seq == needSeq {
					return peer.String(), []byte{}, nil
				}
			default:
				// ignore
			}
		}

		if typ, ok := x.Type.(ipv6.ICMPType); ok && typ == ipv6.ICMPTypeEchoReply {
			b, _ := x.Body.Marshal(1)
			if string(b[4:]) != string(neededBody) || x.Body.(*icmp.Echo).ID != needID {
				continue
			}

			return peer.String(), b[4:], nil
		}
	}
}
