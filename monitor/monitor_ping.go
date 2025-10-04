package monitor

import (
	"context"
	"log/slog"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/syepes/network_exporter/config"
	"github.com/syepes/network_exporter/pkg/common"
	"github.com/syepes/network_exporter/pkg/ping"
	"github.com/syepes/network_exporter/target"
)

// PING manages the goroutines responsible for collecting ICMP data
type PING struct {
	logger            *slog.Logger
	sc                *config.SafeConfig
	resolver          *config.Resolver
	icmpID            *common.IcmpID
	interval          time.Duration
	timeout           time.Duration
	count             int
	payloadSize       int
	ipv6              bool
	maxConcurrentJobs int
	targets           map[string]*target.PING
	mtx               sync.RWMutex
}

// NewPing creates and configures a new Monitoring ICMP instance
func NewPing(logger *slog.Logger, sc *config.SafeConfig, resolver *config.Resolver, icmpID *common.IcmpID, ipv6 bool, maxConcurrentJobs int) *PING {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	return &PING{
		logger:            logger,
		sc:                sc,
		resolver:          resolver,
		icmpID:            icmpID,
		interval:          sc.Cfg.ICMP.Interval.Duration(),
		timeout:           sc.Cfg.ICMP.Timeout.Duration(),
		count:             sc.Cfg.ICMP.Count,
		payloadSize:       sc.Cfg.ICMP.PayloadSize,
		ipv6:              ipv6,
		maxConcurrentJobs: maxConcurrentJobs,
		targets:           make(map[string]*target.PING),
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
	p.logger.Debug("Current Targets", "type", "ICMP", "func", "AddTargets", "count", len(p.targets), "configured", countTargets(p.sc, "ICMP"))

	targetActiveTmp := []string{}
	for _, v := range p.targets {
		targetActiveTmp = common.AppendIfMissing(targetActiveTmp, v.Name())
	}

	targetConfigTmp := []string{}
	for _, v := range p.sc.Cfg.Targets {
		if v.Type == "ICMP" || v.Type == "ICMP+MTR" {
			ipAddrs, err := common.DestAddrs(context.Background(), v.Host, p.resolver.Resolver, p.resolver.Timeout, p.ipv6)
			if err != nil || len(ipAddrs) == 0 {
				p.logger.Warn("Skipping resolve target", "type", "ICMP", "func", "AddTargets", "host", v.Host, "err", err)
			}
			for _, ipAddr := range ipAddrs {
				targetConfigTmp = common.AppendIfMissing(targetConfigTmp, v.Name+" "+ipAddr)
			}
		}
	}

	targetAdd := common.CompareList(targetActiveTmp, targetConfigTmp)
	p.logger.Debug("Target names to add", "type", "ICMP", "func", "AddTargets", "targets", targetAdd)

	for _, targetName := range targetAdd {
		for _, target := range p.sc.Cfg.Targets {
			if target.Type == "ICMP" || target.Type == "ICMP+MTR" {
				ipAddrs, err := common.DestAddrs(context.Background(), target.Host, p.resolver.Resolver, p.resolver.Timeout, p.ipv6)
				if err != nil || len(ipAddrs) == 0 {
					p.logger.Warn("Skipping resolve target", "type", "ICMP", "func", "AddTargets", "host", target.Host, "err", err)
				}

				for _, ipAddr := range ipAddrs {
					if target.Name+" "+ipAddr != targetName {
						continue
					}
					// Add jitter to prevent thundering herd (0-10% of interval)
					jitter := time.Duration(rand.Int63n(int64(p.interval / 10)))
					err := p.AddTargetDelayed(target.Name+" "+ipAddr, target.Host, ipAddr, target.SourceIp, target.Labels.Kv, jitter)
					if err != nil {
						p.logger.Warn("Skipping target", "type", "ICMP", "func", "AddTargets", "host", target.Host, "ip", ipAddr, "err", err)
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
	p.logger.Info("Adding Target", "type", "ICMP", "func", "AddTargetDelayed", "name", name, "host", host, "ip", ip, "delay", startupDelay)

	p.mtx.Lock()
	defer p.mtx.Unlock()

	target, err := target.NewPing(p.logger, p.icmpID, startupDelay, name, host, ip, srcAddr, p.interval, p.timeout, p.count, p.payloadSize, labels, p.ipv6, p.maxConcurrentJobs)
	if err != nil {
		return err
	}
	p.removeTarget(name)
	p.targets[name] = target
	return nil
}

// DelTargets deletes/stops the removed targets from the configuration
func (p *PING) DelTargets() {
	p.logger.Debug("Current Targets", "type", "ICMP", "func", "DelTargets", "count", len(p.targets), "configured", countTargets(p.sc, "ICMP"))

	targetActiveTmp := []string{}
	for _, v := range p.targets {
		if v != nil {
			targetActiveTmp = common.AppendIfMissing(targetActiveTmp, v.Name())
		}
	}

	targetConfigTmp := []string{}
	for _, v := range p.sc.Cfg.Targets {
		if v.Type == "ICMP" || v.Type == "ICMP+MTR" {
			ipAddrs, err := common.DestAddrs(context.Background(), v.Host, p.resolver.Resolver, p.resolver.Timeout, p.ipv6)
			if err != nil || len(ipAddrs) == 0 {
				p.logger.Warn("Skipping resolve target", "type", "ICMP", "func", "DelTargets", "host", v.Host, "err", err)
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
	p.logger.Info("Removing Target", "type", "ICMP", "func", "RemoveTarget", "target", key)
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
	p.logger.Debug("Current Targets", "type", "ICMP", "func", "CheckActiveTargets", "count", len(p.targets), "configured", countTargets(p.sc, "ICMP"))

	targetActiveTmp := make(map[string]string)
	for _, v := range p.targets {
		targetActiveTmp[v.Name()+" "+v.Ip()] = v.Ip()
	}

	for targetName, targetIp := range targetActiveTmp {
		for _, target := range p.sc.Cfg.Targets {
			if target.Type != "ICMP" && target.Type != "ICMP+MTR" {
				continue
			}
			if !strings.HasPrefix(targetName, target.Name+" ") {
				continue
			}
			ipAddrs, err := common.DestAddrs(context.Background(), target.Host, p.resolver.Resolver, p.resolver.Timeout, p.ipv6)
			if err != nil || len(ipAddrs) == 0 {
				return err
			}

			if !common.ContainsString(ipAddrs, targetIp) {
				p.RemoveTarget(targetName)

				for _, ipAddr := range ipAddrs {
					// Add jitter to prevent thundering herd (0-10% of interval)
					jitter := time.Duration(rand.Int63n(int64(p.interval / 10)))
					err := p.AddTargetDelayed(target.Name+" "+ipAddr, target.Host, ipAddr, target.SourceIp, target.Labels.Kv, jitter)
					if err != nil {
						p.logger.Warn("Skipping target", "type", "ICMP", "func", "CheckActiveTargets", "host", target.Host, "ip", ipAddr, "err", err)
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
			// p.logger.Debug("Export metrics", "type", "ICMP", "func", "ExportMetrics", "name", name, "metrics", metrics, "labels", target.Labels())
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
