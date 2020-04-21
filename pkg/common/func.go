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

// CompareList Compare two lists and return a list with the difference
func CompareList(a, b []string) []string {
	var tmpList []string
	ma := make(map[string]bool, len(a))
	for _, ka := range a {
		ma[ka] = true
	}
	for _, kb := range b {
		if !ma[kb] {
			tmpList = append(tmpList, kb)
		}
	}
	return tmpList
}

// AppendIfMissing Append only if the item does not exists in the current list
func AppendIfMissing(slice []string, i string) []string {
	for _, v := range slice {
		if v == i {
			return slice
		}
	}
	return append(slice, i)
}

// HasMapDuplicates Find duplicates in a map keys
func HasMapDuplicates(m map[string]string) bool {
	x := make(map[string]struct{})

	for _, v := range m {
		if _, has := x[v]; has {
			return true
		}
		x[v] = struct{}{}
	}

	return false
}

// HasListDuplicates Find duplicates in a list
func HasListDuplicates(m []string) (string, error) {
	tmp := map[string]bool{}

	for v := range m {
		if tmp[m[v]] == true {
			return m[v], fmt.Errorf("Found duplicated record: %s", m[v])
		}
		tmp[m[v]] = true
	}
	return "", nil
}
