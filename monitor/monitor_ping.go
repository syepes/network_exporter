package monitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/syepes/network_exporter/config"
	"github.com/syepes/network_exporter/pkg/common"
	"github.com/syepes/network_exporter/pkg/ping"
	"github.com/syepes/network_exporter/target"
)

// PING manages the goroutines responsible for collecting ICMP data
type PING struct {
	logger   log.Logger
	sc       *config.SafeConfig
	resolver *config.Resolver
	icmpID   *common.IcmpID
	interval time.Duration
	timeout  time.Duration
	count    int
	targets  map[string]*target.PING
	mtx      sync.RWMutex
}

// NewPing creates and configures a new Monitoring ICMP instance
func NewPing(logger log.Logger, sc *config.SafeConfig, resolver *config.Resolver, icmpID *common.IcmpID) *PING {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &PING{
		logger:   logger,
		sc:       sc,
		resolver: resolver,
		icmpID:   icmpID,
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
	level.Debug(p.logger).Log("type", "ICMP", "func", "AddTargets", "msg", fmt.Sprintf("Current Targets: %d, cfg: %d", len(p.targets), countTargets(p.sc, "ICMP")))

	targetActiveTmp := []string{}
	for _, v := range p.targets {
		targetActiveTmp = common.AppendIfMissing(targetActiveTmp, v.Name())
	}

	targetConfigTmp := []string{}
	for _, v := range p.sc.Cfg.Targets {
		if v.Type == "ICMP" || v.Type == "ICMP+MTR" {
			ipAddrs, err := common.DestAddrs(context.Background(), v.Host, p.resolver.Resolver, p.resolver.Timeout)
			if err != nil || len(ipAddrs) == 0 {
				level.Warn(p.logger).Log("type", "ICMP", "func", "AddTargets", "msg", fmt.Sprintf("Skipping resolve target: %s", v.Host), "err", err)
			}
			for _, ipAddr := range ipAddrs {
				targetConfigTmp = common.AppendIfMissing(targetConfigTmp, v.Name+" "+ipAddr)
			}
		}
	}

	targetAdd := common.CompareList(targetActiveTmp, targetConfigTmp)
	level.Debug(p.logger).Log("type", "ICMP", "func", "AddTargets", "msg", fmt.Sprintf("targetName: %v", targetAdd))

	for _, targetName := range targetAdd {
		for _, target := range p.sc.Cfg.Targets {
			if target.Type == "ICMP" || target.Type == "ICMP+MTR" {
				ipAddrs, err := common.DestAddrs(context.Background(), target.Host, p.resolver.Resolver, p.resolver.Timeout)
				if err != nil || len(ipAddrs) == 0 {
					level.Warn(p.logger).Log("type", "ICMP", "func", "AddTargets", "msg", fmt.Sprintf("Skipping resolve target: %s", target.Host), "err", err)
				}

				for _, ipAddr := range ipAddrs {
					if target.Name+" "+ipAddr != targetName {
						continue
					}
					err := p.AddTarget(target.Name+" "+ipAddr, target.Host, ipAddr, target.SourceIp, target.Labels.Kv)
					if err != nil {
						level.Warn(p.logger).Log("type", "ICMP", "func", "AddTargets", "msg", fmt.Sprintf("Skipping target. Host: %s IP: %s", target.Host, ipAddr), "err", err)
					}
				}
			}
		}
	}
}

// AddTarget adds a target to the monitored list
func (p *PING) AddTarget(name string, host string, ip string, srcAddr string, labels map[string]string) (err error) {
	return p.AddTargetDelayed(name, host, ip, srcAddr, labels, 0)
}

// AddTargetDelayed is AddTarget with a startup delay
func (p *PING) AddTargetDelayed(name string, host string, ip string, srcAddr string, labels map[string]string, startupDelay time.Duration) (err error) {
	level.Info(p.logger).Log("type", "ICMP", "func", "AddTargetDelayed", "msg", fmt.Sprintf("Adding Target: %s (%s/%s) in %s", name, host, ip, startupDelay))

	p.mtx.Lock()
	defer p.mtx.Unlock()

	target, err := target.NewPing(p.logger, p.icmpID, startupDelay, name, host, ip, srcAddr, p.interval, p.timeout, p.count, labels)
	if err != nil {
		return err
	}
	p.removeTarget(name)
	p.targets[name] = target
	return nil
}

// DelTargets deletes/stops the removed targets from the configuration
func (p *PING) DelTargets() {
	level.Debug(p.logger).Log("type", "ICMP", "func", "DelTargets", "msg", fmt.Sprintf("Current Targets: %d, cfg: %d", len(p.targets), countTargets(p.sc, "ICMP")))

	targetActiveTmp := []string{}
	for _, v := range p.targets {
		if v != nil {
			targetActiveTmp = common.AppendIfMissing(targetActiveTmp, v.Name())
		}
	}

	targetConfigTmp := []string{}
	for _, v := range p.sc.Cfg.Targets {
		if v.Type == "ICMP" || v.Type == "ICMP+MTR" {
			ipAddrs, err := common.DestAddrs(context.Background(), v.Host, p.resolver.Resolver, p.resolver.Timeout)
			if err != nil || len(ipAddrs) == 0 {
				level.Warn(p.logger).Log("type", "ICMP", "func", "DelTargets", "msg", fmt.Sprintf("Skipping resolve target: %s", v.Host), "err", err)
			}
			for _, ipAddr := range ipAddrs {
				targetConfigTmp = common.AppendIfMissing(targetConfigTmp, v.Name+" "+ipAddr)
			}
		}
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
func (p *PING) RemoveTarget(key string) {
	level.Info(p.logger).Log("type", "ICMP", "func", "RemoveTarget", "msg", fmt.Sprintf("Removing Target: %s", key))
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

// Read target if IP was changed (DNS record)
func (p *PING) CheckActiveTargets() (err error) {
	level.Debug(p.logger).Log("type", "ICMP", "func", "CheckActiveTargets", "msg", fmt.Sprintf("Current Targets: %d, cfg: %d", len(p.targets), countTargets(p.sc, "ICMP")))

	targetActiveTmp := make(map[string]string)
	for _, v := range p.targets {
		targetActiveTmp[v.Name()+" "+v.Ip()] = v.Ip()
	}

	for targetName, targetIp := range targetActiveTmp {
		for _, target := range p.sc.Cfg.Targets {
			if target.Name != targetName {
				continue
			}
			ipAddrs, err := common.DestAddrs(context.Background(), target.Host, p.resolver.Resolver, p.resolver.Timeout)
			if err != nil || len(ipAddrs) == 0 {
				return err
			}

			if !func(ips []string, target string) bool {
				for _, ip := range ips {
					if ip == target {
						return true
					}
				}
				return false
			}(ipAddrs, targetIp) {

				p.RemoveTarget(targetName + " " + targetIp)

				ipAddrs, err := common.DestAddrs(context.Background(), target.Host, p.resolver.Resolver, p.resolver.Timeout)
				if err != nil || len(ipAddrs) == 0 {
					level.Warn(p.logger).Log("type", "ICMP", "func", "CheckActiveTargets", "msg", fmt.Sprintf("Skipping resolve target: %s", target.Host), "err", err)
				}

				for _, ipAddr := range ipAddrs {
					err := p.AddTarget(target.Name+" "+ipAddr, target.Host, ipAddr, target.SourceIp, target.Labels.Kv)
					if err != nil {
						level.Warn(p.logger).Log("type", "ICMP", "func", "CheckActiveTargets", "msg", fmt.Sprintf("Skipping target. Host: %s IP: %s", target.Host, ipAddr), "err", err)
					}
				}
			}
		}
	}
	return nil
}

// ExportMetrics collects the metrics for each monitored target and returns it as a simple map
func (p *PING) ExportMetrics() map[string]*ping.PingResult {
	m := make(map[string]*ping.PingResult)

	p.mtx.RLock()
	defer p.mtx.RUnlock()

	for _, target := range p.targets {
		name := target.Name()
		metrics := target.Compute()

		if metrics != nil {
			// level.Debug(p.logger).Log("type", "ICMP", "func", "ExportMetrics", "msg", fmt.Sprintf("Name: %s, Metrics: %+v, Labels: %+v", name, metrics, target.Labels()))
			m[name] = metrics
		}
	}
	return m
}

// ExportLabels target labels
func (p *PING) ExportLabels() map[string]map[string]string {
	l := make(map[string]map[string]string)

	p.mtx.RLock()
	defer p.mtx.RUnlock()

	for _, target := range p.targets {
		name := target.Name()
		labels := target.Labels()

		if labels != nil {
			l[name] = labels
		}
	}
	return l
}
