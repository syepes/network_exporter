package mtr

import (
	"bytes"
	"fmt"
	"math"
	"time"

	"github.com/syepes/network_exporter/pkg/common"
	"github.com/syepes/network_exporter/pkg/icmp"
	"github.com/syepes/network_exporter/pkg/tcp"
)

// Mtr Return traceroute object
func Mtr(addr string, srcAddr string, maxHops int, count int, timeout time.Duration, icmpID int, payloadSize int, protocol string, port string, ipv6 bool) (*MtrResult, error) {
	var out MtrResult
	var err error

	options := MtrOptions{}
	options.SetMaxHops(maxHops)
	options.SetCount(count)
	options.SetTimeout(timeout)

	out, err = runMtr(addr, srcAddr, icmpID, &options, payloadSize, protocol, port, ipv6)

	if err == nil {
		if len(out.Hops) == 0 {
			return &out, fmt.Errorf("MTR Expected at least one hop")
		}
	} else {
		return &out, fmt.Errorf("MTR Failed due to an error: %v", err)
	}

	return &out, nil
}

// MtrString Console print traceroute operation
func MtrString(addr string, srcAddr string, maxHops int, count int, timeout time.Duration, icmpID int, payloadSize int, protocol string, port string, ipv6 bool) (result string, err error) {
	options := MtrOptions{}
	options.SetMaxHops(maxHops)
	options.SetCount(count)
	options.SetTimeout(timeout)

	var out MtrResult
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("Start: %v, DestAddr: %v\n", time.Now().Format("2006-01-02 15:04:05"), addr))

	out, err = runMtr(addr, srcAddr, icmpID, &options, payloadSize, protocol, port, ipv6)

	if err == nil {
		if len(out.Hops) == 0 {
			buffer.WriteString("Expected at least one hop\n")
			return buffer.String(), nil
		}
	} else {
		buffer.WriteString(fmt.Sprintf("Failed due to an error: %v\n", err))
		return buffer.String(), err
	}

	buffer.WriteString(fmt.Sprintf("%-3v %-48v  %10v%c  %10v  %10v  %10v  %10v  %10v\n", "", "HOST", "Loss", '%', "Snt", "Last", "Avg", "Best", "Worst"))

	// Format the output of mtr according to the original linux mtr result
	var hopStr string
	var lastHop int
	for index, hop := range out.Hops {
		if hop.Success {
			if hopStr != "" {
				buffer.WriteString(hopStr)
				hopStr = ""
			}

			buffer.WriteString(fmt.Sprintf("%-3d %-48v  %10.1f%c  %10v  %10.2f  %10.2f  %10.2f  %10.2f\n", hop.TTL, hop.AddressTo, hop.Loss, '%', hop.Snt, common.Time2Float(hop.LastTime), common.Time2Float(hop.AvgTime), common.Time2Float(hop.BestTime), common.Time2Float(hop.WorstTime)))
			lastHop = hop.TTL
		} else {
			if index != len(out.Hops)-1 {
				hopStr += fmt.Sprintf("%-3d %-48v  %10.1f%c  %10v  %10.2f  %10.2f  %10.2f  %10.2f\n", hop.TTL, "???", float32(100), '%', int(0), float32(0), float32(0), float32(0), float32(0))
			} else {
				lastHop++
				buffer.WriteString(fmt.Sprintf("%-3d %-48v\n", lastHop, "???"))
			}
		}
	}

	return buffer.String(), nil
}

// MTR
func runMtr(destAddr string, srcAddr string, icmpID int, options *MtrOptions, payloadSize int, protocol string, port string, ipv6 bool) (result MtrResult, err error) {
	result.Hops = []common.IcmpHop{}
	result.DestAddr = destAddr

	// Avoid collisions/interference caused by multiple coroutines initiating mtr
	pid := icmpID
	timeout := options.Timeout()
	mtrReturns := make([]*MtrReturn, options.MaxHops()+1)

	// Verify data packets
	seq := 0
	for snt := 0; snt < options.Count(); snt++ {
		for ttl := 1; ttl < options.MaxHops(); ttl++ {
			if mtrReturns[ttl] == nil {
				mtrReturns[ttl] = &MtrReturn{ttl: ttl, host: "unknown", succSum: 0, success: false, lastTime: time.Duration(0), sumTime: time.Duration(0), bestTime: time.Duration(0), worstTime: time.Duration(0), avgTime: time.Duration(0)}
			}

			var hopReturn common.IcmpReturn
			var err error

			// Use TCP or ICMP based on protocol
			if protocol == "tcp" {
				hopReturn, err = tcp.Traceroute(destAddr, port, srcAddr, ttl, timeout, ipv6)
			} else {
				hopReturn, err = icmp.Icmp(destAddr, srcAddr, ttl, pid, timeout, seq, payloadSize, ipv6)
			}
			seq++
			if err != nil || !hopReturn.Success {
				continue
			}

			mtrReturns[ttl].host = hopReturn.Addr
			mtrReturns[ttl].lastTime = hopReturn.Elapsed
			mtrReturns[ttl].allTime = append(mtrReturns[ttl].allTime, hopReturn.Elapsed)
			mtrReturns[ttl].succSum = mtrReturns[ttl].succSum + 1
			if mtrReturns[ttl].worstTime == time.Duration(0) || hopReturn.Elapsed > mtrReturns[ttl].worstTime {
				mtrReturns[ttl].worstTime = hopReturn.Elapsed
			}
			if mtrReturns[ttl].bestTime == time.Duration(0) || hopReturn.Elapsed < mtrReturns[ttl].bestTime {
				mtrReturns[ttl].bestTime = hopReturn.Elapsed
			}
			mtrReturns[ttl].sumTime += hopReturn.Elapsed
			mtrReturns[ttl].avgTime = mtrReturns[ttl].sumTime / time.Duration(mtrReturns[ttl].succSum)
			mtrReturns[ttl].success = true

			if common.IsEqualIP(hopReturn.Addr, destAddr) {
				break
			}
		}
	}

	for index, mtrReturn := range mtrReturns {
		if index == 0 {
			continue
		}

		if mtrReturn == nil {
			break
		}

		hop := common.IcmpHop{TTL: mtrReturn.ttl, Snt: options.Count()}
		if index != 1 {
			hop.AddressFrom = mtrReturns[index-1].host
		} else {
			hop.AddressFrom = mtrReturn.host
		}
		hop.AddressTo = mtrReturn.host
		hop.Success = mtrReturn.success
		hop.LastTime = mtrReturn.lastTime
		hop.SumTime = mtrReturn.sumTime
		hop.AvgTime = mtrReturn.avgTime
		hop.BestTime = mtrReturn.bestTime
		hop.WorstTime = mtrReturn.worstTime
		hop.SquaredDeviationTime = time.Duration(math.Sqrt(common.TimeSquaredDeviation(mtrReturn.allTime)))
		hop.UncorrectedSDTime = time.Duration(common.TimeUncorrectedDeviation(mtrReturn.allTime))
		hop.CorrectedSDTime = time.Duration(common.TimeCorrectedDeviation(mtrReturn.allTime))
		hop.RangeTime = time.Duration(common.TimeRange(mtrReturn.allTime))

		failSum := options.Count() - mtrReturn.succSum
		hop.SntFail = failSum
		loss := (float64)(failSum) / (float64)(options.Count())
		hop.Loss = float64(loss)

		result.Hops = append(result.Hops, hop)

		if common.IsEqualIP(hop.AddressTo, destAddr) {
			break
		}
	}

	// fmt.Printf("Mtr.result %+v\n", result)
	return result, nil
}
