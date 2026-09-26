package target

import (
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/syepes/network_exporter/pkg/selfmon"
	"github.com/syepes/network_exporter/pkg/tcp"
)

// TCPPort Object
type TCPPort struct {
	logger            *slog.Logger
	name              string
	host              string
	ip                string
	srcAddr           string
	port              string
	interval          time.Duration
	timeout           time.Duration
	maxConcurrentJobs int
	labels            map[string]string
	result            *tcp.TCPPortReturn
	stop              chan struct{}
	wg                sync.WaitGroup
	sync.RWMutex
}

// NewTCPPort starts a new monitoring goroutine
func NewTCPPort(logger *slog.Logger, startupDelay time.Duration, name string, host string, ip string, srcAddr string, port string, interval time.Duration, timeout time.Duration, labels map[string]string, maxConcurrentJobs int) (*TCPPort, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	t := &TCPPort{
		logger:            logger,
		name:              name,
		host:              host,
		ip:                ip,
		srcAddr:           srcAddr,
		port:              port,
		interval:          interval,
		timeout:           timeout,
		maxConcurrentJobs: maxConcurrentJobs,
		labels:            labels,
		stop:              make(chan struct{}),
	}
	t.wg.Add(1)
	go t.run(startupDelay)
	return t, nil
}

func (t *TCPPort) run(startupDelay time.Duration) {
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
	if !acquireSlot(selfmon.TypeTCP, waitChan, t.stop) {
		t.wg.Done()
		return
	}
	go func() {
		t.portCheck()
		<-waitChan
	}()

	tick := time.NewTicker(t.interval)
	defer tick.Stop()

	for {
		select {
		case <-t.stop:
			t.wg.Done()
			return
		case <-tick.C:
			if !acquireSlot(selfmon.TypeTCP, waitChan, t.stop) {
				t.wg.Done()
				return
			}
			go func() {
				t.portCheck()
				<-waitChan
			}()
		}
	}
}

// Stop gracefully stops the monitoring
func (t *TCPPort) Stop() {
	close(t.stop)
	t.wg.Wait()
}

func (t *TCPPort) portCheck() {
	done := selfmon.ProbeStarted(selfmon.TypeTCP)
	data, err := tcp.Port(t.host, t.ip, t.srcAddr, t.port, t.timeout)
	done(err == nil)
	if err != nil {
		t.logger.Error("TCP Port check failed", "type", "TCP", "func", "port", "err", err)
	}

	logDebugResult(t.logger, "TCP Port result", "TCP", "port", data)

	t.Lock()
	defer t.Unlock()
	t.result = data
}

// Compute returns the results of the TCP metrics
func (t *TCPPort) Compute() *tcp.TCPPortReturn {
	t.RLock()
	defer t.RUnlock()

	if t.result == nil {
		return nil
	}
	return t.result
}

// Name returns name
func (t *TCPPort) Name() string {
	t.RLock()
	defer t.RUnlock()
	return t.name
}

// Host returns host
func (t *TCPPort) Host() string {
	t.RLock()
	defer t.RUnlock()
	return t.host
}

// Ip returns ip
func (t *TCPPort) Ip() string {
	t.RLock()
	defer t.RUnlock()
	return t.ip
}

// Labels returns labels
func (t *TCPPort) Labels() map[string]string {
	t.RLock()
	defer t.RUnlock()
	return t.labels
}
