package target

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/syepes/ping_exporter/pkg/ping"
)

// TargetPing is a unit of work
type TargetPing struct {
	logger   log.Logger
	alias    string
	host     string
	interval time.Duration
	timeout  time.Duration
	count    int
	result   *ping.PingReturn
	stop     chan struct{}
	wg       sync.WaitGroup
	sync.RWMutex
}

// NewTargetPing starts a new monitoring goroutine
func NewTargetPing(logger log.Logger, startupDelay time.Duration, alias string, host string, interval time.Duration, timeout time.Duration, count int) (*TargetPing, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	n := &TargetPing{
		logger:   logger,
		alias:    alias,
		host:     host,
		timeout:  timeout,
		interval: interval,
		count:    count,
		stop:     make(chan struct{}),
	}
	n.wg.Add(1)
	go n.run(startupDelay)
	return n, nil
}

func (n *TargetPing) run(startupDelay time.Duration) {
	if startupDelay > 0 {
		select {
		case <-time.After(startupDelay):
		case <-n.stop:
		}
	}

	tick := time.NewTicker(n.interval)
	for {
		select {
		case <-n.stop:
			tick.Stop()
			n.wg.Done()
			return
		case <-tick.C:
			go n.ping()
		}
	}
}

// Stop gracefully stops the monitoring.
func (n *TargetPing) Stop() {
	close(n.stop)
	n.wg.Wait()
}

func (n *TargetPing) ping() {
	mm, err := ping.Ping(n.host, n.count, n.interval, n.timeout)
	if err != nil {
		level.Error(n.logger).Log("msg", fmt.Sprintf("%s", err), "func", "ping")
	}

	/*
		bytes, err2 := json.Marshal(mm)
		if err2 != nil {
			level.Error(n.logger).Log("msg", fmt.Sprintf("%s", err2), "func", "ping")
		}
		level.Debug(n.logger).Log("msg", fmt.Sprintf("%s", string(bytes)), "func", "ping")
	*/

	n.Lock()
	n.result = mm
	n.Unlock()
}

// Compute returns the results of the Ping metrics.
func (n *TargetPing) Compute() *ping.PingReturn {
	n.RLock()
	defer n.RUnlock()

	if n.result == nil {
		return nil
	}
	return n.result
}

// ID returns target ID
func (n *TargetPing) ID() string {
	n.RLock()
	defer n.RUnlock()
	return n.alias + "::" + n.host
}

// Alias returns alias
func (n *TargetPing) Alias() string {
	n.RLock()
	defer n.RUnlock()
	return n.alias
}

// Host returns host
func (n *TargetPing) Host() string {
	n.RLock()
	defer n.RUnlock()
	return n.host
}
