//go:build !windows

package tcp

import (
	"syscall"
)

// setTTLv4 sets the IPv4 TTL socket option on Unix-like systems
func setTTLv4(fd uintptr, ttl int) error {
	return syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_TTL, ttl)
}

// setTTLv6 sets the IPv6 Hop Limit socket option on Unix-like systems
func setTTLv6(fd uintptr, ttl int) error {
	return syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, syscall.IPV6_UNICAST_HOPS, ttl)
}
