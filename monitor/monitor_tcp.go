package monitor

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/syepes/ping_exporter/config"
	"github.com/syepes/ping_exporter/pkg/common"
	"github.com/syepes/ping_exporter/pkg/tcp"
	"github.com/syepes/ping_exporter/target"
)

// TCPPort manages the goroutines responsible for collecting TCP data
type TCPPort struct {
	logger   log.Logger
	sc       *config.SafeConfig
	interval time.Duration
	timeout  time.Duration
	targets  map[string]*target.TCPPort
	mtx      sync.RWMutex
}

// NewPing creates and configures a new Monitoring TCP instance
func NewTCPPort(logger log.Logger, sc *config.SafeConfig) *TCPPort {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &TCPPort{
		logger:   logger,
		sc:       sc,
		interval: sc.Cfg.TCP.Interval.Duration(),
		timeout:  sc.Cfg.TCP.Timeout.Duration(),
		targets:  make(map[string]*target.TCPPort),
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
	level.Debug(p.logger).Log("type", "TCP", "func", "AddTargets", "msg", fmt.Sprintf("Current Targets: %d, cfg: %d", len(p.targets), len(p.sc.Cfg.Targets)))

	targetActiveTmp := []string{}
	for _, v := range p.targets {
		targetActiveTmp = common.AppendIfMissing(targetActiveTmp, v.Name())
	}

	targetConfigTmp := []string{}
	for _, v := range p.sc.Cfg.Targets {
		targetConfigTmp = common.AppendIfMissing(targetConfigTmp, v.Name)
	}

	targetAdd := common.CompareList(targetActiveTmp, targetConfigTmp)
	level.Info(p.logger).Log("type", "TCP", "func", "AddTargets", "msg", fmt.Sprintf("targetName: %v", targetAdd))

	for _, targetName := range targetAdd {
		for _, target := range p.sc.Cfg.Targets {
			if target.Name != targetName {
				continue
			}
			if target.Type == "TCP" {
				conn := strings.Split(target.Host, ":")
				if len(conn) != 2 {
					level.Warn(p.logger).Log("type", "TCP", "func", "AddTargets", "msg", fmt.Sprintf("Skipping target, could not identify host/port: %v", target.Host))
					continue
				}
				p.AddTarget(target.Name, conn[0], conn[1])
			}
		}
	}
}

// AddTarget adds a target to the monitored list
func (p *TCPPort) AddTarget(name string, addr string, port string) (err error) {
	return p.AddTargetDelayed(name, addr, port, 0)
}

// AddTargetDelayed is AddTarget with a startup delay
func (p *TCPPort) AddTargetDelayed(name string, addr string, port string, startupDelay time.Duration) (err error) {
	level.Debug(p.logger).Log("type", "TCP", "func", "AddTargetDelayed", "msg", fmt.Sprintf("Adding Target: %s (%s:%s) in %s", name, addr, port, startupDelay))

	p.mtx.Lock()
	defer p.mtx.Unlock()

	target, err := target.NewTCPPort(p.logger, startupDelay, name, addr, port, p.interval, p.timeout)
	if err != nil {
		return err
	}
	p.removeTarget(name)
	p.targets[name] = target
	return nil
}

// DelTargets deletes/stops the removed targets from the configuration
func (p *TCPPort) DelTargets() {
	level.Debug(p.logger).Log("type", "TCP", "func", "DelTargets", "msg", fmt.Sprintf("Current Targets: %d, cfg: %d", len(p.targets), len(p.sc.Cfg.Targets)))

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
func (p *TCPPort) RemoveTarget(key string) {
	level.Debug(p.logger).Log("type", "TCP", "func", "RemoveTarget", "msg", fmt.Sprintf("Removing Target: %s", key))
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

// Export collects the metrics for each monitored target and returns it as a simple map
func (p *TCPPort) Export() map[string]*tcp.TCPPortReturn {
	m := make(map[string]*tcp.TCPPortReturn)

	p.mtx.RLock()
	defer p.mtx.RUnlock()

	for _, target := range p.targets {
		name := target.Name()
		metrics := target.Compute()
		if metrics != nil {
			// level.Debug(p.logger).Log("type", "TCP", "func", "Export", "msg", fmt.Sprintf("Name: %s, Metrics: %v", name, metrics))
			m[name] = metrics
		}
	}
	return m
}
