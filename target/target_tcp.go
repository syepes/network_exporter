package target

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/syepes/network_exporter/pkg/tcp"
)

// TCPPort Object
type TCPPort struct {
	logger   log.Logger
	name     string
	host     string
	ip       string
	srcAddr  string
	port     string
	interval time.Duration
	timeout  time.Duration
	labels   map[string]string
	result   *tcp.TCPPortReturn
	stop     chan struct{}
	wg       sync.WaitGroup
	sync.RWMutex
}

// NewTCPPort starts a new monitoring goroutine
func NewTCPPort(logger log.Logger, startupDelay time.Duration, name string, host string, ip string, srcAddr string, port string, interval time.Duration, timeout time.Duration, labels map[string]string) (*TCPPort, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	t := &TCPPort{
		logger:   logger,
		name:     name,
		host:     host,
		ip:       ip,
		srcAddr:  srcAddr,
		port:     port,
		interval: interval,
		timeout:  timeout,
		labels:   labels,
		stop:     make(chan struct{}),
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
		}
	}

	tick := time.NewTicker(t.interval)
	for {
		select {
		case <-t.stop:
			tick.Stop()
			t.wg.Done()
			return
		case <-tick.C:
			go t.portCheck()
		}
	}
}

// Stop gracefully stops the monitoring
func (t *TCPPort) Stop() {
	close(t.stop)
	t.wg.Wait()
}

func (t *TCPPort) portCheck() {
	data, err := tcp.Port(t.host, t.ip, t.srcAddr, t.port, t.interval, t.timeout)
	if err != nil {
		level.Error(t.logger).Log("type", "TCP", "func", "port", "msg", fmt.Sprintf("%s", err))
	}

	bytes, err2 := json.Marshal(data)
	if err2 != nil {
		level.Error(t.logger).Log("type", "TCP", "func", "port", "msg", fmt.Sprintf("%s", err2))
	}
	level.Debug(t.logger).Log("type", "TCP", "func", "port", "msg", bytes)

	t.Lock()
	t.result = data
	t.Unlock()
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
