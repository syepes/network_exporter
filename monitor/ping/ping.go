package ping

import (
	"bytes"
	"fmt"
	"math"
	"time"

	"github.com/syepes/ping_exporter/monitor/common"
	"github.com/syepes/ping_exporter/monitor/icmp"
)

// Ping ICMP Operation
func Ping(addr string, count int, timeout time.Duration, interval time.Duration) (*PingReturn, error) {
	var out PingReturn
	var err error

	pingOptions := &PingOptions{}
	pingOptions.SetCount(count)
	pingOptions.SetTimeout(timeout)
	pingOptions.SetInterval(interval)

	// Resolve hostnames
	ipAddrs, err := common.DestAddrs(addr)
	if err != nil || len(ipAddrs) == 0 {
		return nil, fmt.Errorf("Ping Failed due to an error: %v", err)
	}

	out = runPing(ipAddrs[0], pingOptions)

	return &out, nil
}

// PingString ICMP Operation
func PingString(addr string, count int, timeout time.Duration, interval time.Duration) (result string, err error) {
	pingOptions := &PingOptions{}
	pingOptions.SetCount(count)
	pingOptions.SetTimeout(timeout)
	pingOptions.SetInterval(interval)

	// Resolve hostnames
	ipAddrs, err := common.DestAddrs(addr)
	if err != nil || len(ipAddrs) == 0 {
		return result, err
	}

	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("Start %v, PING %v (%v)\n", time.Now().Format("2006-01-02 15:04:05"), addr, ipAddrs[0]))
	begin := time.Now().UnixNano() / 1e6
	pingReturn := runPing(ipAddrs[0], pingOptions)
	end := time.Now().UnixNano() / 1e6

	buffer.WriteString(fmt.Sprintf("%v packets transmitted, %v packet loss, time %vms\n", count, pingReturn.DropRate, end-begin))
	buffer.WriteString(fmt.Sprintf("rtt min/avg/max = %v/%v/%v ms\n", common.Time2Float(pingReturn.WrstTime), common.Time2Float(pingReturn.AvgTime), common.Time2Float(pingReturn.BestTime)))

	result = buffer.String()

	return result, nil
}

func runPing(ipAddr string, option *PingOptions) (pingReturn PingReturn) {
	pingReturn.DestAddr = ipAddr

	pid := common.Goid()
	timeout := option.Timeout()
	interval := option.Interval()
	ttl := defaultTTL
	pingResult := PingResult{}

	// µsPerMs := 1.0 / float64(time.Millisecond)
	data := make([]float64, 0, option.Count())
	seq := 0
	for cnt := 0; cnt < option.Count(); cnt++ {
		icmpReturn, err := icmp.Icmp(ipAddr, ttl, pid, timeout, seq)
		if err != nil || !icmpReturn.Success || !common.IsEqualIP(ipAddr, icmpReturn.Addr) {
			continue
		}

		pingResult.succSum++
		if pingResult.wrstTime == time.Duration(0) || icmpReturn.Elapsed > pingResult.wrstTime {
			pingResult.wrstTime = icmpReturn.Elapsed
		}
		if pingResult.bestTime == time.Duration(0) || icmpReturn.Elapsed < pingResult.bestTime {
			pingResult.bestTime = icmpReturn.Elapsed
		}
		pingResult.allTime += icmpReturn.Elapsed
		pingResult.avgTime = time.Duration((int64)(pingResult.allTime/time.Microsecond)/(int64)(pingResult.succSum)) * time.Microsecond
		pingResult.success = true

		data = append(data, float64(icmpReturn.Elapsed))
		seq++
		time.Sleep(interval)
	}

	if !pingResult.success {
		pingReturn.Success = false
		pingReturn.DropRate = 100.0

		return pingReturn
	}

	pingReturn.Success = pingResult.success
	pingReturn.DropRate = float64(option.Count()-pingResult.succSum) / float64(option.Count())
	pingReturn.AvgTime = pingResult.avgTime
	pingReturn.BestTime = pingResult.bestTime
	pingReturn.WrstTime = pingResult.wrstTime

	size := float64(option.Count()) - float64(option.Count()-pingResult.succSum)
	var sumSquares float64

	for _, rtt := range data {
		sumSquares += math.Pow(rtt-float64(pingReturn.AvgTime), 2)
	}
	stddev := math.Sqrt(sumSquares / size)
	pingReturn.StdDev = stddev

	return pingReturn
}
