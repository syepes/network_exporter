package ping

import (
	"bytes"
	"fmt"
	"math"
	"time"

	"github.com/syepes/network_exporter/pkg/common"
	"github.com/syepes/network_exporter/pkg/icmp"
)

// Ping ICMP Operation
func Ping(addr string, ip string, srcAddr string, count int, timeout time.Duration, icmpID int, payloadSize int, ipv6 bool) (*PingResult, error) {
	var out PingResult

	pingOptions := &PingOptions{}
	pingOptions.SetCount(count)
	pingOptions.SetTimeout(timeout)

	out, err := runPing(addr, ip, srcAddr, icmpID, pingOptions, payloadSize, ipv6)
	if err != nil {
		return &out, err
	}
	return &out, nil
}

// PingString ICMP Operation
func PingString(addr string, ip string, srcAddr string, count int, timeout time.Duration, icmpID int, payloadSize int, ipv6 bool) (result string, err error) {
	pingOptions := &PingOptions{}
	pingOptions.SetCount(count)
	pingOptions.SetTimeout(timeout)

	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("Start %v, PING %v (%v)\n", time.Now().Format("2006-01-02 15:04:05"), addr, addr))
	begin := time.Now().UnixNano() / 1e6
	pingResult, err := runPing(addr, ip, srcAddr, icmpID, pingOptions, payloadSize, ipv6)
	end := time.Now().UnixNano() / 1e6

	buffer.WriteString(fmt.Sprintf("%v packets transmitted, %v packet loss, time %vms\n", count, pingResult.DropRate, end-begin))
	buffer.WriteString(fmt.Sprintf("rtt min/avg/max = %v/%v/%v ms\n", common.Time2Float(pingResult.WorstTime), common.Time2Float(pingResult.AvgTime), common.Time2Float(pingResult.BestTime)))

	result = buffer.String()

	if err != nil {
		return result, err
	}

	return result, nil
}

func runPing(ipAddr string, ip string, srcAddr string, icmpID int, option *PingOptions, payloadSize int, ipv6 bool) (pingResult PingResult, err error) {
	pingResult.DestAddr = ipAddr
	pingResult.DestIp = ip

	// Avoid collisions/interference caused by multiple coroutines initiating mtr
	pid := icmpID
	timeout := option.Timeout()
	ttl := defaultTTL
	pingReturn := PingReturn{}

	seq := 0
	for cnt := 0; cnt < option.Count(); cnt++ {
		icmpReturn, err := icmp.Icmp(ip, srcAddr, ttl, pid, timeout, seq, payloadSize, ipv6)

		if err != nil || !icmpReturn.Success || !common.IsEqualIP(ip, icmpReturn.Addr) {
			continue
		}

		pingReturn.allTime = append(pingReturn.allTime, icmpReturn.Elapsed)

		pingReturn.succSum++
		if pingReturn.worstTime == time.Duration(0) || icmpReturn.Elapsed > pingReturn.worstTime {
			pingReturn.worstTime = icmpReturn.Elapsed
		}
		if pingReturn.bestTime == time.Duration(0) || icmpReturn.Elapsed < pingReturn.bestTime {
			pingReturn.bestTime = icmpReturn.Elapsed
		}
		pingReturn.sumTime += icmpReturn.Elapsed
		pingReturn.avgTime = pingReturn.sumTime / time.Duration(pingReturn.succSum)
		pingReturn.success = true

		seq++
	}

	pingResult.Success = pingReturn.success
	pingResult.DropRate = float64(option.Count()-pingReturn.succSum) / float64(option.Count())
	pingResult.SumTime = pingReturn.sumTime
	pingResult.AvgTime = pingReturn.avgTime
	pingResult.BestTime = pingReturn.bestTime
	pingResult.WorstTime = pingReturn.worstTime
	pingResult.SquaredDeviationTime = time.Duration(math.Sqrt(common.TimeSquaredDeviation(pingReturn.allTime)))
	pingResult.UncorrectedSDTime = time.Duration(common.TimeUncorrectedDeviation(pingReturn.allTime))
	pingResult.CorrectedSDTime = time.Duration(common.TimeCorrectedDeviation(pingReturn.allTime))
	pingResult.RangeTime = time.Duration(common.TimeRange(pingReturn.allTime))
	pingResult.SntSummary = option.Count()
	pingResult.SntFailSummary = option.Count() - pingReturn.succSum
	pingResult.SntTimeSummary = time.Duration(common.TimeRange(pingReturn.allTime))

	return pingResult, nil
}
