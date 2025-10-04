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
	"github.com/syepes/network_exporter/pkg/mtr"
	"github.com/syepes/network_exporter/target"
)

// MTR manages the goroutines responsible for collecting MTR data
type MTR struct {
	logger            *slog.Logger
	sc                *config.SafeConfig
	resolver          *config.Resolver
	icmpID            *common.IcmpID
	interval          time.Duration
	timeout           time.Duration
	maxHops           int
	count             int
	payloadSize       int
	protocol          string
	tcpPort           string
	ipv6              bool
	maxConcurrentJobs int
	targets           map[string]*target.MTR
	mtx               sync.RWMutex
}

// NewMTR creates and configures a new Monitoring MTR instance
func NewMTR(logger *slog.Logger, sc *config.SafeConfig, resolver *config.Resolver, icmpID *common.IcmpID, ipv6 bool, maxConcurrentJobs int) *MTR {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	return &MTR{
		logger:            logger,
		sc:                sc,
		resolver:          resolver,
		icmpID:            icmpID,
		interval:          sc.Cfg.MTR.Interval.Duration(),
		timeout:           sc.Cfg.MTR.Timeout.Duration(),
		maxHops:           sc.Cfg.MTR.MaxHops,
		count:             sc.Cfg.MTR.Count,
		payloadSize:       sc.Cfg.MTR.PayloadSize,
		protocol:          sc.Cfg.MTR.Protocol,
		tcpPort:           sc.Cfg.MTR.TcpPort,
		ipv6:              ipv6,
		maxConcurrentJobs: maxConcurrentJobs,
		targets:           make(map[string]*target.MTR),
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
	p.logger.Debug("Current Targets", "type", "MTR", "func", "AddTargets", "count", len(p.targets), "configured", countTargets(p.sc, "MTR"))

	targetActiveTmp := []string{}
	for _, v := range p.targets {
		targetActiveTmp = common.AppendIfMissing(targetActiveTmp, v.Name())
	}

	targetConfigTmp := []string{}
	for _, v := range p.sc.Cfg.Targets {
		if v.Type == "MTR" || v.Type == "ICMP+MTR" {
			targetConfigTmp = common.AppendIfMissing(targetConfigTmp, v.Name)
		}
	}

	targetAdd := common.CompareList(targetActiveTmp, targetConfigTmp)
	p.logger.Debug("Target names to add", "type", "MTR", "func", "AddTargets", "targets", targetAdd)

	for _, targetName := range targetAdd {
		for _, target := range p.sc.Cfg.Targets {
			if target.Name != targetName {
				continue
			}

			if target.Type == "MTR" || target.Type == "ICMP+MTR" {
				// Add jitter to prevent thundering herd (0-10% of interval)
				jitter := time.Duration(rand.Int63n(int64(p.interval / 10)))
				err := p.AddTargetDelayed(target.Name, target.Host, target.SourceIp, target.Labels.Kv, jitter)
				if err != nil {
					p.logger.Warn("Skipping target", "type", "MTR", "func", "AddTargets", "host", target.Host, "err", err)
				}
			}
		}
	}
}

// AddTarget adds a target to the monitored list
func (p *MTR) AddTarget(name string, host string, srcAddr string, labels map[string]string) (err error) {
	return p.AddTargetDelayed(name, host, srcAddr, labels, 0)
}

// AddTargetDelayed is AddTarget with a startup delay
func (p *MTR) AddTargetDelayed(name string, host string, srcAddr string, labels map[string]string, startupDelay time.Duration) (err error) {
	p.logger.Info("Adding Target", "type", "MTR", "func", "AddTargetDelayed", "name", name, "host", host, "delay", startupDelay)

	p.mtx.Lock()
	defer p.mtx.Unlock()

	// Parse port from host if specified (for TCP protocol)
	targetHost := host
	targetPort := p.tcpPort // Use default port from config
	if p.protocol == "tcp" && strings.Contains(host, ":") {
		// Extract port from host string (e.g., "example.com:443")
		parts := strings.Split(host, ":")
		if len(parts) == 2 {
			targetHost = parts[0]
			targetPort = parts[1]
		}
	}

	// Resolve hostnames
	ipAddrs, err := common.DestAddrs(context.Background(), targetHost, p.resolver.Resolver, p.resolver.Timeout, p.ipv6)
	if err != nil || len(ipAddrs) == 0 {
		return err
	}

	target, err := target.NewMTR(p.logger, p.icmpID, startupDelay, name, ipAddrs[0], srcAddr, p.interval, p.timeout, p.maxHops, p.count, p.payloadSize, p.protocol, targetPort, labels, p.ipv6, p.maxConcurrentJobs)
	if err != nil {
		return err
	}
	p.removeTarget(name)
	p.targets[name] = target
	return nil
}

// DelTargets deletes/stops the removed targets from the configuration
func (p *MTR) DelTargets() {
	p.logger.Debug("Current Targets", "type", "MTR", "func", "DelTargets", "count", len(p.targets), "configured", countTargets(p.sc, "MTR"))

	targetActiveTmp := []string{}
	for _, v := range p.targets {
		if v != nil {
			targetActiveTmp = common.AppendIfMissing(targetActiveTmp, v.Name())
		}
	}

	targetConfigTmp := []string{}
	for _, v := range p.sc.Cfg.Targets {
		if v.Type == "MTR" || v.Type == "ICMP+MTR" {
			targetConfigTmp = common.AppendIfMissing(targetConfigTmp, v.Name)
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
func (p *MTR) RemoveTarget(key string) {
	p.logger.Info("Removing Target", "type", "MTR", "func", "RemoveTarget", "target", key)
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

// Read target if IP was changed (DNS record)
func (p *MTR) CheckActiveTargets() (err error) {
	p.logger.Debug("Current Targets", "type", "MTR", "func", "CheckActiveTargets", "count", len(p.targets), "configured", countTargets(p.sc, "MTR"))

	targetActiveTmp := make(map[string]string)
	for _, v := range p.targets {
		targetActiveTmp[v.Name()] = v.Host()
	}

	for targetName, targetIp := range targetActiveTmp {
		for _, target := range p.sc.Cfg.Targets {
			if target.Name != targetName {
				continue
			}
			ipAddrs, err := common.DestAddrs(context.Background(), target.Host, p.resolver.Resolver, p.resolver.Timeout, p.ipv6)
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

				p.RemoveTarget(targetName)
				// Add jitter to prevent thundering herd (0-10% of interval)
				jitter := time.Duration(rand.Int63n(int64(p.interval / 10)))
				err := p.AddTargetDelayed(target.Name, target.Host, target.SourceIp, target.Labels.Kv, jitter)
				if err != nil {
					p.logger.Warn("Skipping target", "type", "MTR", "func", "CheckActiveTargets", "host", target.Host, "err", err)
				}
			}
		}
	}
	return nil
}

// ExportMetrics collects the metrics for each monitored target and returns it as a simple map
func (p *MTR) ExportMetrics() map[string]*mtr.MtrResult {
	m := make(map[string]*mtr.MtrResult)

	p.mtx.RLock()
	defer p.mtx.RUnlock()

	for _, target := range p.targets {
		name := target.Name()
		metrics := target.Compute()

		if metrics != nil {
			// p.logger.Debug("Export metrics", "type", "MTR", "func", "ExportMetrics", "name", name, "metrics", metrics, "labels", target.Labels())
			m[name] = metrics
		}
	}
	return m
}

// ExportLabels target labels
func (p *MTR) ExportLabels() map[string]map[string]string {
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
