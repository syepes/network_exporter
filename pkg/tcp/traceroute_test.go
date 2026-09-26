package tcp

import (
	"encoding/binary"
	"net"
	"testing"

	"golang.org/x/net/ipv6"
)

// quotedV4 builds the payload an ICMPv4 Time Exceeded quotes back: a 20-byte
// IPv4 header (no options) followed by the first 8 bytes of the original TCP
// segment (source port, destination port, sequence number).
func quotedV4(src, dst net.IP, srcPort, dstPort uint16) []byte {
	b := make([]byte, 20+8)
	b[0] = 0x45 // version 4, IHL 5 (20 bytes)
	binary.BigEndian.PutUint16(b[2:4], 40)
	b[8] = 64 // TTL
	b[9] = 6  // protocol TCP
	copy(b[12:16], src.To4())
	copy(b[16:20], dst.To4())
	binary.BigEndian.PutUint16(b[20:22], srcPort)
	binary.BigEndian.PutUint16(b[22:24], dstPort)
	return b
}

// quotedV6 builds the payload an ICMPv6 Time Exceeded quotes back: a 40-byte
// IPv6 header (no extension headers) followed by the first bytes of the original
// TCP segment.
func quotedV6(src, dst net.IP, srcPort, dstPort uint16) []byte {
	b := make([]byte, ipv6.HeaderLen+8)
	b[0] = 0x60                           // version 6
	binary.BigEndian.PutUint16(b[4:6], 8) // payload length
	b[6] = 6                              // next header TCP
	b[7] = 64                             // hop limit
	copy(b[8:24], src.To16())
	copy(b[24:40], dst.To16())
	binary.BigEndian.PutUint16(b[40:42], srcPort)
	binary.BigEndian.PutUint16(b[42:44], dstPort)
	return b
}

func TestICMPMatchesFlowV4(t *testing.T) {
	src := net.ParseIP("192.0.2.1")
	dst := net.ParseIP("198.51.100.10")
	other := net.ParseIP("203.0.113.7")
	data := quotedV4(src, dst, 40000, 443)

	tests := []struct {
		name    string
		wantSrc net.IP
		wantDst net.IP
		port    int
		want    bool
	}{
		{"exact match", src, dst, 443, true},
		{"match ignoring src when unset", nil, dst, 443, true},
		{"wrong dst ip", src, other, 443, false},
		{"wrong dst port", src, dst, 80, false},
		{"wrong src ip", other, dst, 443, false},
		{"port 0 skips port check", src, dst, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := icmpMatchesFlowV4(data, tc.wantSrc, tc.wantDst, tc.port); got != tc.want {
				t.Fatalf("icmpMatchesFlowV4 = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("truncated payload without ports is rejected when a port is required", func(t *testing.T) {
		if icmpMatchesFlowV4(quotedV4(src, dst, 40000, 443)[:22], src, dst, 443) {
			t.Fatalf("expected truncated payload to fail the port check")
		}
	})

	t.Run("garbage payload is rejected", func(t *testing.T) {
		if icmpMatchesFlowV4([]byte{0x00, 0x01, 0x02}, src, dst, 443) {
			t.Fatalf("expected unparseable payload to be rejected")
		}
	})
}

func TestICMPMatchesFlowV6(t *testing.T) {
	src := net.ParseIP("2001:db8::1")
	dst := net.ParseIP("2001:db8::2")
	other := net.ParseIP("2001:db8::99")
	data := quotedV6(src, dst, 40000, 443)

	tests := []struct {
		name    string
		wantSrc net.IP
		wantDst net.IP
		port    int
		want    bool
	}{
		{"exact match", src, dst, 443, true},
		{"match ignoring src when unset", nil, dst, 443, true},
		{"wrong dst ip", src, other, 443, false},
		{"wrong dst port", src, dst, 80, false},
		{"wrong src ip", other, dst, 443, false},
		{"port 0 skips port check", src, dst, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := icmpMatchesFlowV6(data, tc.wantSrc, tc.wantDst, tc.port); got != tc.want {
				t.Fatalf("icmpMatchesFlowV6 = %v, want %v", got, tc.want)
			}
		})
	}
}
