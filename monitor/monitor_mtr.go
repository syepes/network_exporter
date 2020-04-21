package monitor

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/syepes/ping_exporter/pkg/mtr"
	"github.com/syepes/ping_exporter/target"
)

// MonitorMTR manages the goroutines responsible for collecting MTR data
type MonitorMTR struct {
	logger   log.Logger
	interval time.Duration
	timeout  time.Duration
	maxHops  int
	sntSize  int
	targets  map[string]*target.TargetMTR
	mtx      sync.RWMutex
}

// New creates and configures a new MTR instance
func NewMTR(logger log.Logger, interval time.Duration, timeout time.Duration, maxHops int, sntSize int) *MonitorMTR {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &MonitorMTR{
		logger:   logger,
		interval: interval,
		timeout:  timeout,
		maxHops:  maxHops,
		sntSize:  sntSize,
		targets:  make(map[string]*target.TargetMTR),
	}
}

// Stop brings the monitoring gracefully to a halt
func (p *MonitorMTR) Stop() {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for id := range p.targets {
		p.removeTarget(id)
	}
}

// AddTarget adds a target to the monitored list
func (p *MonitorMTR) AddTarget(alias string, addr string) (err error) {
	return p.AddTargetDelayed(alias, addr, 0)
}

// AddTargetDelayed is AddTarget with a startup delay
func (p *MonitorMTR) AddTargetDelayed(alias string, addr string, startupDelay time.Duration) (err error) {
	level.Debug(p.logger).Log("msg", fmt.Sprintf("Adding Target: %s (%s)", alias, addr), "type", "MTR", "func", "AddTargetDelayed")

	p.mtx.Lock()
	defer p.mtx.Unlock()

	target, err := target.NewTargetMTR(p.logger, startupDelay, alias, addr, p.interval, p.timeout, p.maxHops, p.sntSize)
	if err != nil {
		return err
	}
	p.removeTarget(target.ID())
	p.targets[target.ID()] = target
	return nil
}

// RemoveTarget removes a target from the monitoring list
func (p *MonitorMTR) RemoveTarget(key string) {
	level.Debug(p.logger).Log("msg", fmt.Sprintf("Removing Target: %s", key), "type", "MTR", "func", "RemoveTarget")
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.removeTarget(key)
}

// Stops monitoring a target and removes it from the list (if the list includes the target)
func (p *MonitorMTR) removeTarget(key string) {
	target, found := p.targets[key]
	if !found {
		return
	}
	target.Stop()
	delete(p.targets, key)
}

// TargetList removes a target from the monitoring list
func (p *MonitorMTR) TargetList() map[string]*target.TargetMTR {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	return p.targets
}

// Export collects the metrics for each monitored target and returns it as a simple map
func (p *MonitorMTR) Export() map[string]*mtr.MtrResult {
	m := make(map[string]*mtr.MtrResult)

	p.mtx.RLock()
	defer p.mtx.RUnlock()

	for _, target := range p.targets {
		alias := target.Alias()
		metrics := target.Compute()
		if metrics != nil {
			// level.Debug(p.logger).Log("msg", fmt.Sprintf("METRICS: %v", metrics), "type", "MTR", "func", "Export")
			m[alias] = metrics
		}
	}
	return m
}
