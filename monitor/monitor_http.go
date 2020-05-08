package monitor

import (
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/syepes/ping_exporter/config"
	"github.com/syepes/ping_exporter/pkg/common"
	"github.com/syepes/ping_exporter/pkg/http"
	"github.com/syepes/ping_exporter/target"
)

// HTTPGet manages the goroutines responsible for collecting HTTPGet data
type HTTPGet struct {
	logger   log.Logger
	sc       *config.SafeConfig
	resolver *net.Resolver
	interval time.Duration
	timeout  time.Duration
	targets  map[string]*target.HTTPGet
	mtx      sync.RWMutex
}

// NewHTTPGet creates and configures a new Monitoring HTTPGet instance
func NewHTTPGet(logger log.Logger, sc *config.SafeConfig, resolver *net.Resolver) *HTTPGet {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &HTTPGet{
		logger:   logger,
		sc:       sc,
		resolver: resolver,
		interval: sc.Cfg.HTTPGet.Interval.Duration(),
		timeout:  sc.Cfg.HTTPGet.Timeout.Duration(),
		targets:  make(map[string]*target.HTTPGet),
	}
}

// Stop brings the monitoring gracefully to a halt
func (p *HTTPGet) Stop() {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for id := range p.targets {
		p.removeTarget(id)
	}
}

// AddTargets adds newly added targets from the configuration
func (p *HTTPGet) AddTargets() {
	level.Debug(p.logger).Log("type", "HTTPGet", "func", "AddTargets", "msg", fmt.Sprintf("Current Targets: %d, cfg: %d", len(p.targets), len(p.sc.Cfg.Targets)))

	targetActiveTmp := []string{}
	for _, v := range p.targets {
		targetActiveTmp = common.AppendIfMissing(targetActiveTmp, v.Name())
	}

	targetConfigTmp := []string{}
	for _, v := range p.sc.Cfg.Targets {
		targetConfigTmp = common.AppendIfMissing(targetConfigTmp, v.Name)
	}

	targetAdd := common.CompareList(targetActiveTmp, targetConfigTmp)
	// level.Debug(p.logger).Log("type", "HTTPGet", "func", "AddTargets", "msg", fmt.Sprintf("targetName: %v", targetAdd))

	for _, targetName := range targetAdd {
		for _, target := range p.sc.Cfg.Targets {
			if target.Name != targetName {
				continue
			}
			if target.Type == "HTTPGet" {
				if target.Proxy != "" {
					err := p.AddTarget(target.Name, target.Host, target.Proxy)
					if err != nil {
						level.Warn(p.logger).Log("type", "HTTPGet", "func", "AddTargets", "msg", fmt.Sprintf("Skipping target: %s", target.Host), "err", err)
					}
				} else {
					err := p.AddTarget(target.Name, target.Host, "")
					if err != nil {
						level.Warn(p.logger).Log("type", "HTTPGet", "func", "AddTargets", "msg", fmt.Sprintf("Skipping target: %s", target.Host), "err", err)
					}
				}
			}
		}
	}
}

// AddTarget adds a target to the monitored list
func (p *HTTPGet) AddTarget(name string, url string, proxy string) (err error) {
	return p.AddTargetDelayed(name, url, proxy, 0)
}

// AddTargetDelayed is AddTarget with a startup delay
func (p *HTTPGet) AddTargetDelayed(name string, urlStr string, proxy string, startupDelay time.Duration) (err error) {
	if proxy != "" {
		level.Debug(p.logger).Log("type", "HTTPGet", "func", "AddTargetDelayed", "msg", fmt.Sprintf("Adding Target: %s (%s) with proxy (%s) in %s", name, urlStr, proxy, startupDelay))
	} else {
		level.Debug(p.logger).Log("type", "HTTPGet", "func", "AddTargetDelayed", "msg", fmt.Sprintf("Adding Target: %s (%s) in %s", name, urlStr, startupDelay))
	}

	p.mtx.Lock()
	defer p.mtx.Unlock()

	// Check URL
	dURL, err := url.ParseRequestURI(urlStr)
	if err != nil {
		return err
	}

	// Check Proxy URL
	if proxy != "" {
		_, err := url.ParseRequestURI(proxy)
		if err != nil {
			return err
		}
	}

	target, err := target.NewHTTPGet(p.logger, startupDelay, name, dURL.String(), proxy, p.interval, p.timeout)
	if err != nil {
		return err
	}
	p.removeTarget(name)
	p.targets[name] = target
	return nil
}

// DelTargets deletes/stops the removed targets from the configuration
func (p *HTTPGet) DelTargets() {
	level.Debug(p.logger).Log("type", "HTTPGet", "func", "DelTargets", "msg", fmt.Sprintf("Current Targets: %d, cfg: %d", len(p.targets), len(p.sc.Cfg.Targets)))

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
func (p *HTTPGet) RemoveTarget(key string) {
	level.Debug(p.logger).Log("type", "HTTPGet", "func", "RemoveTarget", "msg", fmt.Sprintf("Removing Target: %s", key))
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.removeTarget(key)
}

// Stops monitoring a target and removes it from the list (if the list includes the target)
func (p *HTTPGet) removeTarget(key string) {
	target, found := p.targets[key]
	if !found {
		return
	}
	target.Stop()
	delete(p.targets, key)
}

// Export collects the metrics for each monitored target and returns it as a simple map
func (p *HTTPGet) Export() map[string]*http.HTTPReturn {
	m := make(map[string]*http.HTTPReturn)

	p.mtx.RLock()
	defer p.mtx.RUnlock()

	for _, target := range p.targets {
		name := target.Name()
		metrics := target.Compute()
		if metrics != nil {
			// level.Debug(p.logger).Log("type", "HTTPGet", "func", "Export", "msg", fmt.Sprintf("Name: %s, Metrics: %+v", name, metrics))
			m[name] = metrics
		}
	}
	return m
}
