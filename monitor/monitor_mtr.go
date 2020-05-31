package monitor

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/syepes/network_exporter/config"
	"github.com/syepes/network_exporter/pkg/common"
	"github.com/syepes/network_exporter/pkg/mtr"
	"github.com/syepes/network_exporter/target"
)

// MTR manages the goroutines responsible for collecting MTR data
type MTR struct {
	logger   log.Logger
	sc       *config.SafeConfig
	resolver *net.Resolver
	icmpID   *common.IcmpID
	interval time.Duration
	timeout  time.Duration
	maxHops  int
	count    int
	targets  map[string]*target.MTR
	mtx      sync.RWMutex
}

// NewMTR creates and configures a new Monitoring MTR instance
func NewMTR(logger log.Logger, sc *config.SafeConfig, resolver *net.Resolver, icmpID *common.IcmpID) *MTR {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &MTR{
		logger:   logger,
		sc:       sc,
		resolver: resolver,
		icmpID:   icmpID,
		interval: sc.Cfg.MTR.Interval.Duration(),
		timeout:  sc.Cfg.MTR.Timeout.Duration(),
		maxHops:  sc.Cfg.MTR.MaxHops,
		count:    sc.Cfg.MTR.Count,
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
	level.Debug(p.logger).Log("type", "MTR", "func", "AddTargets", "msg", fmt.Sprintf("Current Targets: %d, cfg: %d", len(p.targets), countTargets(p.sc, "MTR")))

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
				err := p.AddTarget(target.Name, target.Host)
				if err != nil {
					level.Warn(p.logger).Log("type", "MTR", "func", "AddTargets", "msg", fmt.Sprintf("Skipping target: %s", target.Host), "err", err)
				}
			}
		}
	}
}

// AddTarget adds a target to the monitored list
func (p *MTR) AddTarget(name string, host string) (err error) {
	return p.AddTargetDelayed(name, host, 0)
}

// AddTargetDelayed is AddTarget with a startup delay
func (p *MTR) AddTargetDelayed(name string, host string, startupDelay time.Duration) (err error) {
	level.Debug(p.logger).Log("type", "MTR", "func", "AddTargetDelayed", "msg", fmt.Sprintf("Adding Target: %s (%s) in %s", name, host, startupDelay))

	p.mtx.Lock()
	defer p.mtx.Unlock()

	// Resolve hostnames
	ipAddrs, err := common.DestAddrs(host, p.resolver)
	if err != nil || len(ipAddrs) == 0 {
		return err
	}

	target, err := target.NewMTR(p.logger, p.icmpID, startupDelay, name, ipAddrs[0], p.interval, p.timeout, p.maxHops, p.count)
	if err != nil {
		return err
	}
	p.removeTarget(name)
	p.targets[name] = target
	return nil
}

// DelTargets deletes/stops the removed targets from the configuration
func (p *MTR) DelTargets() {
	level.Debug(p.logger).Log("type", "MTR", "func", "DelTargets", "msg", fmt.Sprintf("Current Targets: %d, cfg: %d", len(p.targets), countTargets(p.sc, "MTR")))

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
			// level.Debug(p.logger).Log("type", "MTR", "func", "Export", "msg", fmt.Sprintf("Name: %s, Metrics: %+v", name, metrics))
			m[name] = metrics
		}
	}
	return m
}
