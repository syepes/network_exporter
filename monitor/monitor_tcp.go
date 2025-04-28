package monitor

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/syepes/network_exporter/config"
	"github.com/syepes/network_exporter/pkg/common"
	"github.com/syepes/network_exporter/pkg/tcp"
	"github.com/syepes/network_exporter/target"
)

// TCPPort manages the goroutines responsible for collecting TCP data
type TCPPort struct {
	logger    *slog.Logger
	sc        *config.SafeConfig
	resolver  *config.Resolver
	interval  time.Duration
	timeout   time.Duration
	targets   map[string]*target.TCPPort
	mtx       sync.RWMutex
}

// NewTCPPort creates and configures a new Monitoring TCP instance
func NewTCPPort(logger *slog.Logger, sc *config.SafeConfig, resolver *config.Resolver) *TCPPort {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	return &TCPPort{
		logger:    logger,
		sc:        sc,
		resolver:  resolver,
		interval:  sc.Cfg.TCP.Interval.Duration(),
		timeout:   sc.Cfg.TCP.Timeout.Duration(),
		targets:   make(map[string]*target.TCPPort),
	}
}

// Stop brings the monitoring gracefully to a halt
func (p *TCPPort) Stop() {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for id := range p.targets {
		p.removeTarget(id)
	}
}

// AddTargets adds newly added targets from the configuration
func (p *TCPPort) AddTargets() {
	p.logger.Debug("Current Targets", "type", "TCP", "func", "AddTargets", "count", len(p.targets), "configured", countTargets(p.sc, "TCP"))

	targetActiveTmp := []string{}
	for _, v := range p.targets {
		targetActiveTmp = common.AppendIfMissing(targetActiveTmp, v.Name())
	}

	targetConfigTmp := []string{}
	for _, v := range p.sc.Cfg.Targets {
		if v.Type == "TCP" {
			conn := strings.Split(v.Host, ":")
			if len(conn) != 2 {
				p.logger.Warn("Skipping target, could not identify host", "type", "TCP", "func", "AddTargets", "host", v.Host, "name", v.Name)
				continue
			}
			ipAddrs, err := common.DestAddrs(context.Background(), conn[0], p.resolver.Resolver, p.resolver.Timeout)
			if err != nil || len(ipAddrs) == 0 {
				p.logger.Warn("Skipping resolve target", "type", "TCP", "func", "AddTargets", "host", v.Host, "err", err)
			}
			for _, ipAddr := range ipAddrs {
				targetConfigTmp = common.AppendIfMissing(targetConfigTmp, v.Name+" "+ipAddr)
			}
		}
	}

	targetAdd := common.CompareList(targetActiveTmp, targetConfigTmp)
	p.logger.Debug("Target names to add", "type", "TCP", "func", "AddTargets", "targets", targetAdd)

	for _, targetName := range targetAdd {
		for _, target := range p.sc.Cfg.Targets {
			if target.Type == "TCP" {
				conn := strings.Split(target.Host, ":")
				if len(conn) != 2 {
					p.logger.Warn("Skipping target, could not identify host", "type", "TCP", "func", "AddTargets", "host", target.Host, "name", target.Name)
					continue
				}
				ipAddrs, err := common.DestAddrs(context.Background(), conn[0], p.resolver.Resolver, p.resolver.Timeout)
				if err != nil || len(ipAddrs) == 0 {
					p.logger.Warn("Skipping resolve target", "type", "TCP", "func", "AddTargets", "name", target.Name, "err", err)
				}
				for _, ipAddr := range ipAddrs {
					if target.Name+" "+ipAddr != targetName {
						continue
					}
					conn := strings.Split(target.Host, ":")
					if len(conn) != 2 {
						p.logger.Warn("Skipping target, could not identify host", "type", "TCP", "func", "AddTargets", "host", target.Host, "name", target.Name)
						continue
					}
					ipAddrs, err := common.DestAddrs(context.Background(), conn[0], p.resolver.Resolver, p.resolver.Timeout)
					if err != nil || len(ipAddrs) == 0 {
						p.logger.Warn("Skipping resolve target", "type", "TCP", "func", "AddTargets", "host", target.Host, "err", err)
					}
					for _, ipAddr := range ipAddrs {
						err := p.AddTarget(target.Name+" "+ipAddr, conn[0], ipAddr, target.SourceIp, conn[1], target.Labels.Kv)
						if err != nil {
							p.logger.Warn("Skipping target", "type", "TCP", "func", "AddTargets", "host", target.Host, "ip", ipAddr, "err", err)
						}
					}
				}
			}
		}
	}
}

// AddTarget adds a target to the monitored list
func (p *TCPPort) AddTarget(name string, host string, ip string, srcAddr string, port string, labels map[string]string) (err error) {
	return p.AddTargetDelayed(name, host, ip, srcAddr, port, labels, 0)
}

// AddTargetDelayed is AddTarget with a startup delay
func (p *TCPPort) AddTargetDelayed(name string, host string, ip string, srcAddr string, port string, labels map[string]string, startupDelay time.Duration) (err error) {
	p.logger.Info("Adding Target", "type", "TCP", "func", "AddTargetDelayed", "name", name, "host", host, "ip", ip, "port", port, "delay", startupDelay)

	p.mtx.Lock()
	defer p.mtx.Unlock()

	target, err := target.NewTCPPort(p.logger, startupDelay, name, host, ip, srcAddr, port, p.interval, p.timeout, labels)
	if err != nil {
		return err
	}
	p.removeTarget(name)
	p.targets[name] = target
	return nil
}

// DelTargets deletes/stops the removed targets from the configuration
func (p *TCPPort) DelTargets() {
	p.logger.Debug("Current Targets", "type", "TCP", "func", "DelTargets", "count", len(p.targets), "configured", countTargets(p.sc, "TCP"))

	targetActiveTmp := []string{}
	for _, v := range p.targets {
		if v != nil {
			targetActiveTmp = common.AppendIfMissing(targetActiveTmp, v.Name())
		}
	}

	targetConfigTmp := []string{}
	for _, v := range p.sc.Cfg.Targets {
		if v.Type == "TCP" {
			conn := strings.Split(v.Host, ":")
			if len(conn) != 2 {
				p.logger.Warn("Skipping target, could not identify host", "type", "TCP", "func", "DelTargets", "host", v.Host, "name", v.Name)
				continue
			}
			ipAddrs, err := common.DestAddrs(context.Background(), conn[0], p.resolver.Resolver, p.resolver.Timeout)
			if err != nil || len(ipAddrs) == 0 {
				p.logger.Warn("Skipping resolve target", "type", "TCP", "func", "DelTargets", "host", v.Host, "err", err)
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
func (p *TCPPort) RemoveTarget(key string) {
	p.logger.Info("Removing Target", "type", "TCP", "func", "RemoveTarget", "target", key)
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.removeTarget(key)
}

// Stops monitoring a target and removes it from the list (if the list includes the target)
func (p *TCPPort) removeTarget(key string) {
	target, found := p.targets[key]
	if !found {
		return
	}
	target.Stop()
	delete(p.targets, key)
}

// Read target if IP was changed (DNS record)
func (p *TCPPort) CheckActiveTargets() (err error) {
	p.logger.Debug("Current Targets", "type", "TCP", "func", "CheckActiveTargets", "count", len(p.targets), "configured", countTargets(p.sc, "TCP"))

	targetActiveTmp := make(map[string]string)
	for _, v := range p.targets {
		targetActiveTmp[v.Name()+" "+v.Ip()] = v.Ip()
	}

	for targetName, targetIp := range targetActiveTmp {
		for _, target := range p.sc.Cfg.Targets {
			if target.Name != targetName {
				continue
			}
			ipAddrs, err := common.DestAddrs(context.Background(), strings.Split(target.Host, ":")[0], p.resolver.Resolver, p.resolver.Timeout)
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

				conn := strings.Split(target.Host, ":")
				if len(conn) != 2 {
					p.logger.Warn("Skipping target, could not identify host", "type", "TCP", "func", "CheckActiveTargets", "host", target.Host, "name", target.Name)
					continue
				}
				for _, ipAddr := range ipAddrs {
					err := p.AddTarget(target.Name+" "+ipAddr, conn[0], ipAddr, target.SourceIp, conn[1], target.Labels.Kv)
					if err != nil {
						p.logger.Warn("Skipping target", "type", "TCP", "func", "CheckActiveTargets", "host", target.Host, "err", err)
					}
				}
			}
		}
	}
	return nil
}

// ExportMetrics collects the metrics for each monitored target and returns it as a simple map
func (p *TCPPort) ExportMetrics() map[string]*tcp.TCPPortReturn {
	m := make(map[string]*tcp.TCPPortReturn)

	p.mtx.RLock()
	defer p.mtx.RUnlock()

	for _, target := range p.targets {
		name := target.Name()
		metrics := target.Compute()

		if metrics != nil {
			// p.logger.Debug("Export metrics", "type", "TCP", "func", "ExportMetrics", "name", name, "metrics", metrics, "labels", target.Labels())
			m[name] = metrics
		}
	}
	return m
}

// ExportLabels target labels
func (p *TCPPort) ExportLabels() map[string]map[string]string {
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
