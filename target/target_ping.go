package target

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/syepes/network_exporter/pkg/common"
	"github.com/syepes/network_exporter/pkg/ping"
)

const MaxConcurrentJobs = 3 // DEPRECATED: Use maxConcurrentJobs parameter instead

// PING Object
type PING struct {
	logger            *slog.Logger
	icmpID            *common.IcmpID
	name              string
	host              string
	ip                string
	srcAddr           string
	interval          time.Duration
	timeout           time.Duration
	count             int
	payloadSize       int
	ipv6              bool
	maxConcurrentJobs int
	labels            map[string]string
	result            *ping.PingResult
	stop              chan struct{}
	wg                sync.WaitGroup
	sync.RWMutex
}

// NewPing starts a new monitoring goroutine
func NewPing(logger *slog.Logger, icmpID *common.IcmpID, startupDelay time.Duration, name string, host string, ip string, srcAddr string, interval time.Duration, timeout time.Duration, count int, payloadSize int, labels map[string]string, ipv6 bool, maxConcurrentJobs int) (*PING, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	t := &PING{
		logger:            logger,
		icmpID:            icmpID,
		name:              name,
		host:              host,
		ip:                ip,
		srcAddr:           srcAddr,
		interval:          interval,
		timeout:           timeout,
		count:             count,
		payloadSize:       payloadSize,
		ipv6:              ipv6,
		maxConcurrentJobs: maxConcurrentJobs,
		labels:            labels,
		stop:              make(chan struct{}),
		result:            &ping.PingResult{},
	}
	t.wg.Add(1)
	go t.run(startupDelay)
	return t, nil
}

func (t *PING) run(startupDelay time.Duration) {
	if startupDelay > 0 {
		select {
		case <-time.After(startupDelay):
		case <-t.stop:
			t.wg.Done()
			return
		}
	}

	waitChan := make(chan struct{}, t.maxConcurrentJobs)

	// Execute first probe immediately (after jitter delay)
	// This ensures targets start probing as quickly as possible
	select {
	case <-t.stop:
		t.wg.Done()
		return
	default:
		waitChan <- struct{}{}
		go func() {
			t.ping()
			<-waitChan
		}()
	}

	tick := time.NewTicker(t.interval)
	defer tick.Stop()

	for {
		select {
		case <-t.stop:
			t.wg.Done()
			return
		case <-tick.C:
			waitChan <- struct{}{}
			go func() {
				t.ping()
				<-waitChan
			}()
		}
	}
}

// Stop gracefully stops the monitoring
func (t *PING) Stop() {
	close(t.stop)
	t.wg.Wait()
}

func (t *PING) ping() {
	icmpID := int(t.icmpID.Get())
	data, err := ping.Ping(t.host, t.ip, t.srcAddr, t.count, t.timeout, icmpID, t.payloadSize, t.ipv6)
	if err != nil {
		t.logger.Error("Ping failed", "type", "ICMP", "func", "ping", "err", err)
	}

	t.Lock()
	defer t.Unlock()
	data.SntSummary += t.result.SntSummary
	data.SntFailSummary += t.result.SntFailSummary
	data.SntTimeSummary += t.result.SntTimeSummary
	t.result = data

	bytes, err2 := json.Marshal(t.result)
	if err2 != nil {
		t.logger.Error("Failed to marshal result", "type", "ICMP", "func", "ping", "err", err2)
	}
	t.logger.Debug("Ping result", "type", "ICMP", "func", "ping", "result", string(bytes))
}

// Compute returns the results of the Ping metrics
func (t *PING) Compute() *ping.PingResult {
	t.RLock()
	defer t.RUnlock()

	if t.result == nil {
		return nil
	}
	return t.result
}

// Name returns name
func (t *PING) Name() string {
	t.RLock()
	defer t.RUnlock()
	return t.name
}

// Host returns host
func (t *PING) Host() string {
	t.RLock()
	defer t.RUnlock()
	return t.host
}

// Ip returns ip
func (t *PING) Ip() string {
	t.RLock()
	defer t.RUnlock()
	return t.ip
}

// Labels returns labels
func (t *PING) Labels() map[string]string {
	t.RLock()
	defer t.RUnlock()
	return t.labels
}
