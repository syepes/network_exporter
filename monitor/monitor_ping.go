package monitor

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/syepes/ping_exporter/config"
	"github.com/syepes/ping_exporter/pkg/common"
	"github.com/syepes/ping_exporter/pkg/ping"
	"github.com/syepes/ping_exporter/target"
)

// PING manages the goroutines responsible for collecting ICMP data
type PING struct {
	sc       *config.SafeConfig
	logger   log.Logger
	interval time.Duration
	timeout  time.Duration
	count    int
	targets  map[string]*target.PING
	mtx      sync.RWMutex
}

// NewPing creates and configures a new Monitoring ICMP instance
func NewPing(logger log.Logger, sc *config.SafeConfig) *PING {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &PING{
		sc:       sc,
		logger:   logger,
		interval: sc.Cfg.ICMP.Interval.Duration(),
		timeout:  sc.Cfg.ICMP.Timeout.Duration(),
		count:    sc.Cfg.ICMP.Count,
		targets:  make(map[string]*target.PING),
	}
}

// Stop brings the monitoring gracefully to a halt
func (p *PING) Stop() {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for id := range p.targets {
		p.removeTarget(id)
	}
}

// AddTargets adds newly added targets from the configuration
func (p *PING) AddTargets() {
	level.Debug(p.logger).Log("type", "ICMP", "func", "AddTargets", "msg", fmt.Sprintf("Current Targets: %d, cfg: %d", len(p.targets), len(p.sc.Cfg.Targets)))

	targetActiveTmp := []string{}
	for _, v := range p.targets {
		targetActiveTmp = common.AppendIfMissing(targetActiveTmp, v.ID())
	}

	targetConfigTmp := []string{}
	for _, v := range p.sc.Cfg.Targets {
		targetConfigTmp = common.AppendIfMissing(targetConfigTmp, v.Name+"::"+v.Host)
	}

	targetAdd := common.CompareList(targetActiveTmp, targetConfigTmp)
	level.Info(p.logger).Log("type", "ICMP", "func", "AddTargets", "msg", fmt.Sprintf("targetID: %v", targetAdd))

	for _, targetID := range targetAdd {
		for _, host := range p.sc.Cfg.Targets {
			if host.Name+"::"+host.Host != targetID {
				continue
			}
			if host.Type == "ICMP" || host.Type == "ICMP+MTR" {
				p.AddTarget(host.Name, host.Host)
			}
		}
	}
}

// AddTarget adds a target to the monitored list
func (p *PING) AddTarget(name string, addr string) (err error) {
	return p.AddTargetDelayed(name, addr, 0)
}

// AddTargetDelayed is AddTarget with a startup delay
func (p *PING) AddTargetDelayed(name string, addr string, startupDelay time.Duration) (err error) {
	level.Debug(p.logger).Log("type", "ICMP", "func", "AddTargetDelayed", "msg", fmt.Sprintf("Adding Target: %s (%s)", name, addr))

	p.mtx.Lock()
	defer p.mtx.Unlock()

	target, err := target.NewPing(p.logger, startupDelay, name, addr, p.interval, p.timeout, p.count)
	if err != nil {
		return err
	}
	p.removeTarget(target.ID())
	p.targets[target.ID()] = target
	return nil
}

// DelTargets deletes/stops the removed targets from the configuration
func (p *PING) DelTargets() {
	level.Debug(p.logger).Log("type", "ICMP", "func", "DelTargets", "msg", fmt.Sprintf("Current Targets: %d, cfg: %d", len(p.targets), len(p.sc.Cfg.Targets)))

	targetActiveTmp := []string{}
	for _, v := range p.targets {
		if v != nil {
			targetActiveTmp = common.AppendIfMissing(targetActiveTmp, v.ID())
		}
	}

	targetConfigTmp := []string{}
	for _, v := range p.sc.Cfg.Targets {
		targetConfigTmp = common.AppendIfMissing(targetConfigTmp, v.Name+"::"+v.Host)
	}

	targetDelete := common.CompareList(targetConfigTmp, targetActiveTmp)
	for _, targetID := range targetDelete {
		for _, t := range p.targets {
			if t == nil {
				continue
			}
			if t.ID() == targetID {
				p.RemoveTarget(targetID)
			}
		}
	}
}

// RemoveTarget removes a target from the monitoring list
func (p *PING) RemoveTarget(key string) {
	level.Debug(p.logger).Log("type", "ICMP", "func", "RemoveTarget", "msg", fmt.Sprintf("Removing Target: %s", key))
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.removeTarget(key)
}

// Stops monitoring a target and removes it from the list (if the list includes the target)
func (p *PING) removeTarget(key string) {
	target, found := p.targets[key]
	if !found {
		return
	}
	target.Stop()
	delete(p.targets, key)
}

// Export collects the metrics for each monitored target and returns it as a simple map
func (p *PING) Export() map[string]*ping.PingReturn {
	m := make(map[string]*ping.PingReturn)

	p.mtx.RLock()
	defer p.mtx.RUnlock()

	for _, target := range p.targets {
		name := target.Name()
		metrics := target.Compute()
		if metrics != nil {
			// level.Debug(p.logger).Log("msg", fmt.Sprintf("ID: %s, METRICS: %v", id, metrics), "type", "ICMP", "func", "Export")
			m[name] = metrics
		}
	}
	return m
}
