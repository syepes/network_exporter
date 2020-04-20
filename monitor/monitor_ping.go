package monitor

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/syepes/ping_exporter/pkg/ping"
	"github.com/syepes/ping_exporter/target"
)

// MonitorPing manages the goroutines responsible for collecting ICMP data
type MonitorPing struct {
	logger   log.Logger
	interval time.Duration
	timeout  time.Duration
	count    int
	targets  map[string]*target.TargetPing
	mtx      sync.RWMutex
}

// New creates and configures a new ICMP instance
func New(logger log.Logger, interval time.Duration, timeout time.Duration, count int) *MonitorPing {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &MonitorPing{
		logger:   logger,
		interval: interval,
		timeout:  timeout,
		count:    count,
		targets:  make(map[string]*target.TargetPing),
	}
}

// Stop brings the monitoring gracefully to a halt
func (p *MonitorPing) Stop() {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for id := range p.targets {
		p.removeTarget(id)
	}
}

// AddTarget adds a target to the monitored list
func (p *MonitorPing) AddTarget(alias string, addr string) (err error) {
	return p.AddTargetDelayed(alias, addr, 0)
}

// AddTargetDelayed is AddTarget with a startup delay
func (p *MonitorPing) AddTargetDelayed(alias string, addr string, startupDelay time.Duration) (err error) {
	level.Debug(p.logger).Log("msg", fmt.Sprintf("Adding Target: %s (%s)", alias, addr), "type", "ICMP", "func", "AddTargetDelayed")

	p.mtx.Lock()
	defer p.mtx.Unlock()

	target, err := target.NewTargetPing(p.logger, startupDelay, alias, addr, p.interval, p.timeout, p.count)
	if err != nil {
		return err
	}
	p.removeTarget(target.ID())
	p.targets[target.ID()] = target
	return nil
}

// RemoveTarget removes a target from the monitoring list
func (p *MonitorPing) RemoveTarget(key string) {
	level.Debug(p.logger).Log("msg", fmt.Sprintf("Removing Target: %s", key), "type", "ICMP", "func", "RemoveTarget")
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.removeTarget(key)
}

// Stops monitoring a target and removes it from the list (if the list includes the target)
func (p *MonitorPing) removeTarget(key string) {
	target, found := p.targets[key]
	if !found {
		return
	}
	target.Stop()
	delete(p.targets, key)
}

// TargetList removes a target from the monitoring list
func (p *MonitorPing) TargetList() map[string]*target.TargetPing {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	return p.targets
}

// Export collects the metrics for each monitored target and returns it as a simple map
func (p *MonitorPing) Export() map[string]*ping.PingReturn {
	m := make(map[string]*ping.PingReturn)

	p.mtx.RLock()
	defer p.mtx.RUnlock()

	for _, target := range p.targets {
		alias := target.Alias()
		metrics := target.Compute()
		if metrics != nil {
			// level.Debug(p.logger).Log("msg", fmt.Sprintf("ID: %s, METRICS: %v", id, metrics), "type", "ICMP", "func", "Export")
			m[alias] = metrics
		}
	}
	return m
}
