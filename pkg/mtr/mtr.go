package mtr

import (
	"bytes"
	"fmt"
	"time"

	"github.com/syepes/ping_exporter/pkg/common"
	"github.com/syepes/ping_exporter/pkg/icmp"
)

// Mtr Return traceroute object
func Mtr(host string, maxHops int, sntSize int, timeout time.Duration) (*MtrResult, error) {
	var out MtrResult
	var err error

	options := MtrOptions{}
	options.SetMaxHops(maxHops)
	options.SetSntSize(sntSize)
	options.SetTimeout(timeout)

	// Resolve hostnames
	ipAddrs, err := common.DestAddrs(host)
	if err != nil || len(ipAddrs) == 0 {
		return nil, fmt.Errorf("MTR Failed due to an error: %v", err)
	}
	out, err = runMtr(ipAddrs[0], &options)

	if err == nil {
		if len(out.Hops) == 0 {
			return nil, fmt.Errorf("MTR Expected at least one hop")
		}
	} else {
		return nil, fmt.Errorf("MTR Failed due to an error: %v", err)
	}

	return &out, nil
}

// MtrString Console print traceroute operation
func MtrString(host string, maxHops int, sntSize int, timeout time.Duration) (result string, err error) {
	options := MtrOptions{}
	options.SetMaxHops(maxHops)
	options.SetSntSize(sntSize)
	options.SetTimeout(timeout)

	var out MtrResult
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("Start: %v, DestAddr: %v\n", time.Now().Format("2006-01-02 15:04:05"), host))

	// Resolve hostnames
	ipAddrs, err := common.DestAddrs(host)
	if err != nil || len(ipAddrs) == 0 {
		return buffer.String(), fmt.Errorf("MTR Failed due to an error: %v", err)
	}
	out, err = runMtr(ipAddrs[0], &options)

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
func runMtr(destAddr string, options *MtrOptions) (result MtrResult, err error) {
	result.Hops = []common.IcmpHop{}
	result.DestAddr = destAddr

	// Avoid interference caused by multiple coroutines initiating mtr
	pid := common.Goid()
	timeout := time.Duration(options.Timeout()) * time.Millisecond
	mtrReturns := make([]*MtrReturn, options.MaxHops()+1)

	// Verify data packets
	seq := 0
	for snt := 0; snt < options.SntSize(); snt++ {
		for ttl := 1; ttl < options.MaxHops(); ttl++ {
			if mtrReturns[ttl] == nil {
				mtrReturns[ttl] = &MtrReturn{ttl: ttl, host: "???", succSum: 0, success: false, lastTime: time.Duration(0), sumTime: time.Duration(0), bestTime: time.Duration(0), worstTime: time.Duration(0), avgTime: time.Duration(0)}
			}

			hopReturn, err := icmp.Icmp(destAddr, ttl, pid, timeout, seq)
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
			mtrReturns[ttl].avgTime = time.Duration((int64)(mtrReturns[ttl].sumTime/time.Microsecond)/(int64)(mtrReturns[ttl].succSum)) * time.Microsecond
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

		hop := common.IcmpHop{TTL: mtrReturn.ttl, Snt: options.SntSize()}
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
		hop.StdDev = common.StdDev(mtrReturn.allTime, mtrReturn.avgTime)
		failSum := options.SntSize() - mtrReturn.succSum
		loss := (float32)(failSum) / (float32)(options.SntSize()) * 100
		hop.Loss = float32(loss)

		result.Hops = append(result.Hops, hop)

		if common.IsEqualIP(hop.AddressTo, destAddr) {
			break
		}
	}
	// level.Debug(logger).Log("addr", dest.IP.String(), "msg", fmt.Sprintf("TracerouteResult: %+v", result), "func", "mtr")

	return result, nil
}
