package common

import (
	"fmt"
	"net"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// DestAddrs resolve the hostname to all it'ss IP's
func DestAddrs(addr string) ([]string, error) {
	addrs, err := net.LookupHost(addr)
	if err != nil {
		return nil, err
	}

	// Validate IPs
	ipAddrs := make([]string, 0)
	for _, addr := range addrs {
		ipAddr, err := net.ResolveIPAddr("ip", addr)
		if err != nil {
			continue
		}

		ipAddrs = append(ipAddrs, ipAddr.IP.String())
	}

	return ipAddrs, nil
}

// Goid Get specific ID for coroutine
func Goid() int {
	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("panic recover:panic info: %v\n", err)
		}
	}()

	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	idField := strings.Fields(strings.TrimPrefix(string(buf[:n]), "goroutine "))[0]
	id, err := strconv.Atoi(idField)
	if err != nil {
		panic(fmt.Sprintf("cannot get goroutine id: %v", err))
	}
	return id
}

// IsEqualIP IP Comparison
func IsEqualIP(ips1, ips2 string) bool {
	ip1 := net.ParseIP(ips1)
	if ip1 == nil {
		return false
	}

	ip2 := net.ParseIP(ips2)
	if ip2 == nil {
		return false
	}

	if ip1.String() != ip2.String() {
		return false
	}

	return true
}

// Time2Float Convert time to float32
func Time2Float(t time.Duration) float32 {
	return (float32)(t/time.Microsecond) / float32(1000)
}
