package monitor

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/syepes/ping_exporter/config"
	"github.com/syepes/ping_exporter/pkg/common"
	"github.com/syepes/ping_exporter/pkg/mtr"
	"github.com/syepes/ping_exporter/target"
)

// MTR manages the goroutines responsible for collecting MTR data
type MTR struct {
	logger   log.Logger
	sc       *config.SafeConfig
	interval time.Duration
	timeout  time.Duration
	maxHops  int
	sntSize  int
	targets  map[string]*target.MTR
	mtx      sync.RWMutex
}

// NewMTR creates and configures a new Monitoring MTR instance
func NewMTR(logger log.Logger, sc *config.SafeConfig) *MTR {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &MTR{
		logger:   logger,
		sc:       sc,
		interval: sc.Cfg.MTR.Interval.Duration(),
		timeout:  sc.Cfg.MTR.Timeout.Duration(),
		maxHops:  sc.Cfg.MTR.MaxHops,
		sntSize:  sc.Cfg.MTR.SntSize,
		targets:  make(map[string]*target.MTR),
	}
}

// Stop brings the monitoring gracefully to a halt
func (p *MTR) Stop() {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for id := range p.targets {
		p.removeTarget(id)
	}
}

// AddTargets adds newly added targets from the configuration
func (p *MTR) AddTargets() {
	level.Debug(p.logger).Log("type", "MTR", "func", "AddTargets", "msg", fmt.Sprintf("Current Targets: %d, cfg: %d", len(p.targets), len(p.sc.Cfg.Targets)))

	targetActiveTmp := []string{}
	for _, v := range p.targets {
		targetActiveTmp = common.AppendIfMissing(targetActiveTmp, v.Name())
	}

	targetConfigTmp := []string{}
	for _, v := range p.sc.Cfg.Targets {
		targetConfigTmp = common.AppendIfMissing(targetConfigTmp, v.Name)
	}

	targetAdd := common.CompareList(targetActiveTmp, targetConfigTmp)
	// level.Debug(p.logger).Log("type", "MTR", "func", "AddTargets", "msg", fmt.Sprintf("targetName: %v", targetAdd))

	for _, targetName := range targetAdd {
		for _, target := range p.sc.Cfg.Targets {
			if target.Name != targetName {
				continue
			}

			if target.Type == "MTR" || target.Type == "ICMP+MTR" {
				p.AddTarget(target.Name, target.Host)
			}
		}
	}
}

// AddTarget adds a target to the monitored list
func (p *MTR) AddTarget(name string, addr string) (err error) {
	return p.AddTargetDelayed(name, addr, 0)
}

// AddTargetDelayed is AddTarget with a startup delay
func (p *MTR) AddTargetDelayed(name string, addr string, startupDelay time.Duration) (err error) {
	level.Debug(p.logger).Log("type", "MTR", "func", "AddTargetDelayed", "msg", fmt.Sprintf("Adding Target: %s (%s) in %s", name, addr, startupDelay))

	p.mtx.Lock()
	defer p.mtx.Unlock()

	target, err := target.NewMTR(p.logger, startupDelay, name, addr, p.interval, p.timeout, p.maxHops, p.sntSize)
	if err != nil {
		return err
	}
	p.removeTarget(name)
	p.targets[name] = target
	return nil
}

// DelTargets deletes/stops the removed targets from the configuration
func (p *MTR) DelTargets() {
	level.Debug(p.logger).Log("type", "MTR", "func", "DelTargets", "msg", fmt.Sprintf("Current Targets: %d, cfg: %d", len(p.targets), len(p.sc.Cfg.Targets)))

	targetActiveTmp := []string{}
	for _, v := range p.targets {
		if v != nil {
			targetActiveTmp = common.AppendIfMissing(targetActiveTmp, v.Name())
		}
	}

	targetConfigTmp := []string{}
	for _, v := range p.sc.Cfg.Targets {
		targetConfigTmp = common.AppendIfMissing(targetConfigTmp, v.Name)
	}

	targetDelete := common.CompareList(targetConfigTmp, targetActiveTmp)
	for _, targetName := range targetDelete {
		for _, t := range p.targets {
			if t == nil {
				continue
			}
			if t.Name() == targetName {
				p.RemoveTarget(targetName)
			}
		}
	}
}

// RemoveTarget removes a target from the monitoring list
func (p *MTR) RemoveTarget(key string) {
	level.Debug(p.logger).Log("type", "MTR", "func", "RemoveTarget", "msg", fmt.Sprintf("Removing Target: %s", key))
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.removeTarget(key)
}

// Stops monitoring a target and removes it from the list (if the list includes the target)
func (p *MTR) removeTarget(key string) {
	target, found := p.targets[key]
	if !found {
		return
	}
	target.Stop()
	delete(p.targets, key)
}

// Export collects the metrics for each monitored target and returns it as a simple map
func (p *MTR) Export() map[string]*mtr.MtrResult {
	m := make(map[string]*mtr.MtrResult)

	p.mtx.RLock()
	defer p.mtx.RUnlock()

	for _, target := range p.targets {
		name := target.Name()
		metrics := target.Compute()
		if metrics != nil {
			// level.Debug(p.logger).Log("type", "MTR", "func", "Export", "msg", fmt.Sprintf("Name: %s, Metrics: %v", name, metrics))
			m[name] = metrics
		}
	}
	return m
}
