package common

import (
	"context"
	"fmt"
	"math"
	"net"
	"strings"
	"time"
)

func SrvRecordCheck(record string) bool {
	record_split := strings.Split(record, ".")
	if strings.HasPrefix(record_split[0], "_") && strings.HasPrefix(record_split[1], "_") {
		return true
	} else {
		return false
	}
}

func SrvRecordHosts(record string) ([]string, error) {
	record_split := strings.Split(record, ".")
	service :=  record_split[0][1:]
	proto := record_split[1][1:]

	_, members, err := net.LookupSRV(service, proto, strings.Join(record_split[2:],"."))
	if err != nil {
		return nil, fmt.Errorf("resolving target: %v", err)
	}
	hosts := []string{}
	if proto == "tcp" {
		for _, host := range members {
			hosts = append(hosts,  fmt.Sprintf("%s:%d", host.Target[:len(host.Target) - 1], host.Port))
		}
	} else {
		for _, host := range members {
			hosts = append(hosts, host.Target[:len(host.Target) - 1])
		}
	}

	return hosts, nil
}

// DestAddrs resolve the hostname to all it'ss IP's
func DestAddrs(host string, resolver *net.Resolver) ([]string, error) {
	ipAddrs := make([]string, 0)

	addrs, err := resolver.LookupIPAddr(context.Background(), host)
	if err != nil {
		return nil, fmt.Errorf("Resolving target: %v", err)
	}

	// Validate IPs
	for _, addr := range addrs {
		ipAddr, err := net.ResolveIPAddr("ip", addr.IP.String())
		if err != nil {
			continue
		}
		ipAddrs = append(ipAddrs, ipAddr.IP.String())
	}

	return ipAddrs, nil
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

// TimeRange finds the range of a slice of durations
func TimeRange(values []time.Duration) time.Duration {
	if len(values) <= 1 {
		return time.Duration(0)
	}
	min := values[0]
	max := time.Duration(0)
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return max - min
}

// TimeAverage Calculates the average of a slice of durations
func TimeAverage(values []time.Duration) float64 {
	l := len(values)
	if l <= 0 {
		return float64(0.0)
	}
	s := time.Duration(0)
	for _, d := range values {
		s += d
	}
	return float64(s) / float64(l)
}

// TimeSquaredDeviation Calculates the squared deviation
func TimeSquaredDeviation(values []time.Duration) float64 {
	avg := TimeAverage(values)
	sd := 0.0
	for _, v := range values {
		sd += math.Pow((float64(v) - float64(avg)), 2.0)
	}
	return sd
}

// TimeUncorrectedDeviation Calculates standard deviation without correction
func TimeUncorrectedDeviation(values []time.Duration) float64 {
	if len(values) == 0 {
		return 0.0
	}
	sd := TimeSquaredDeviation(values)
	return math.Sqrt(sd / float64(len(values)))
}

// TimeCorrectedDeviation Calculates standard deviation using Bessel's correction which uses n-1 in the SD formula to correct bias of small sample size
func TimeCorrectedDeviation(values []time.Duration) float64 {
	sd := TimeSquaredDeviation(values)
	return math.Sqrt(sd / (float64(len(values)) - 1))
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
		if tmp[m[v]] {
			return m[v], fmt.Errorf("Found duplicated record: %s", m[v])
		}
		tmp[m[v]] = true
	}
	return "", nil
}
