package target

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/syepes/ping_exporter/pkg/mtr"
)

// TargetMTR is a unit of work
type TargetMTR struct {
	logger   log.Logger
	alias    string
	host     string
	interval time.Duration
	timeout  time.Duration
	maxHops  int
	sntSize  int
	result   *mtr.MtrResult
	stop     chan struct{}
	wg       sync.WaitGroup
	sync.RWMutex
}

// NewTargetMTR starts a new monitoring goroutine
func NewTargetMTR(logger log.Logger, startupDelay time.Duration, alias string, host string, interval time.Duration, timeout time.Duration, maxHops int, sntSize int) (*TargetMTR, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	n := &TargetMTR{
		logger:   logger,
		alias:    alias,
		host:     host,
		interval: interval,
		timeout:  timeout,
		maxHops:  maxHops,
		sntSize:  sntSize,
		stop:     make(chan struct{}),
	}
	n.wg.Add(1)
	go n.run(startupDelay)
	return n, nil
}

func (n *TargetMTR) run(startupDelay time.Duration) {
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
			go n.mtr()
		}
	}
}

// Stop gracefully stops the monitoring.
func (n *TargetMTR) Stop() {
	close(n.stop)
	n.wg.Wait()
}

func (n *TargetMTR) mtr() {
	mm, err := mtr.Mtr(n.host, n.maxHops, n.sntSize, n.timeout)
	if err != nil {
		level.Error(n.logger).Log("msg", fmt.Sprintf("%s", err), "func", "mtr")
	}

	/*
		bytes, err2 := json.Marshal(mm)
		if err2 != nil {
			level.Error(n.logger).Log("msg", fmt.Sprintf("%s", err2), "func", "mtr")
		}
		level.Debug(n.logger).Log("msg", fmt.Sprintf("%s", string(bytes)), "func", "mtr")
	*/

	n.Lock()
	n.result = mm
	n.Unlock()
}

// Compute returns the results of the Ping metrics.
func (n *TargetMTR) Compute() *mtr.MtrResult {
	n.RLock()
	defer n.RUnlock()

	if n.result == nil {
		return nil
	}
	return n.result
}

// ID returns target ID
func (n *TargetMTR) ID() string {
	n.RLock()
	defer n.RUnlock()
	return n.alias + "::" + n.host
}

// Alias returns alias
func (n *TargetMTR) Alias() string {
	n.RLock()
	defer n.RUnlock()
	return n.alias
}

// Host returns host
func (n *TargetMTR) Host() string {
	n.RLock()
	defer n.RUnlock()
	return n.host
}
