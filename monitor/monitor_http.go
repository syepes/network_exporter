package monitor

import (
	"log/slog"
	"math/rand"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/syepes/network_exporter/config"
	"github.com/syepes/network_exporter/pkg/common"
	"github.com/syepes/network_exporter/pkg/http"
	"github.com/syepes/network_exporter/target"
)

// HTTPGet manages the goroutines responsible for collecting HTTPGet data
type HTTPGet struct {
	logger            *slog.Logger
	sc                *config.SafeConfig
	resolver          *config.Resolver
	interval          time.Duration
	timeout           time.Duration
	maxConcurrentJobs int
	targets           map[string]*target.HTTPGet
	mtx               sync.RWMutex
}

// NewHTTPGet creates and configures a new Monitoring HTTPGet instance
func NewHTTPGet(logger *slog.Logger, sc *config.SafeConfig, resolver *config.Resolver, maxConcurrentJobs int) *HTTPGet {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	return &HTTPGet{
		logger:            logger,
		sc:                sc,
		resolver:          resolver,
		interval:          sc.Cfg.HTTPGet.Interval.Duration(),
		timeout:           sc.Cfg.HTTPGet.Timeout.Duration(),
		maxConcurrentJobs: maxConcurrentJobs,
		targets:           make(map[string]*target.HTTPGet),
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
	p.logger.Debug("Current Targets", "type", "HTTPGet", "func", "AddTargets", "count", len(p.targets), "configured", countTargets(p.sc, "HTTPGet"))

	targetActiveTmp := []string{}
	for _, v := range p.targets {
		targetActiveTmp = common.AppendIfMissing(targetActiveTmp, v.Name())
	}

	targetConfigTmp := []string{}
	for _, v := range p.sc.Cfg.Targets {
		if v.Type == "HTTPGet" {
			targetConfigTmp = common.AppendIfMissing(targetConfigTmp, v.Name)
		}
	}

	targetAdd := common.CompareList(targetActiveTmp, targetConfigTmp)
	p.logger.Debug("Target names to add", "type", "HTTPGet", "func", "AddTargets", "targets", targetAdd)

	for _, targetName := range targetAdd {
		for _, target := range p.sc.Cfg.Targets {
			if target.Name != targetName {
				continue
			}
			if target.Type == "HTTPGet" {
				// Add jitter to prevent thundering herd (0-10% of interval)
				jitter := time.Duration(rand.Int63n(int64(p.interval / 10)))
				if target.Proxy != "" {
					err := p.AddTargetDelayed(target.Name, target.Host, target.SourceIp, target.Proxy, target.Labels.Kv, jitter)
					if err != nil {
						p.logger.Warn("Skipping target", "type", "HTTPGet", "func", "AddTargets", "host", target.Host, "name", target.Name, "err", err)
					}
				} else {
					err := p.AddTargetDelayed(target.Name, target.Host, target.SourceIp, "", target.Labels.Kv, jitter)
					if err != nil {
						p.logger.Warn("Skipping target", "type", "HTTPGet", "func", "AddTargets", "host", target.Host, "name", target.Name, "err", err)
					}
				}
			}
		}
	}
}

// AddTarget adds a target to the monitored list
func (p *HTTPGet) AddTarget(name string, url string, srcAddr string, proxy string, labels map[string]string) (err error) {
	return p.AddTargetDelayed(name, url, srcAddr, proxy, labels, 0)
}

// AddTargetDelayed is AddTarget with a startup delay
func (p *HTTPGet) AddTargetDelayed(name string, urlStr string, srcAddr string, proxy string, labels map[string]string, startupDelay time.Duration) (err error) {
	if proxy != "" {
		p.logger.Info("Adding Target", "type", "HTTPGet", "func", "AddTargetDelayed", "name", name, "url", urlStr, "proxy", proxy, "delay", startupDelay)
	} else {
		p.logger.Info("Adding Target", "type", "HTTPGet", "func", "AddTargetDelayed", "name", name, "url", urlStr, "delay", startupDelay)
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

	target, err := target.NewHTTPGet(p.logger, startupDelay, name, dURL.String(), srcAddr, proxy, p.interval, p.timeout, labels, p.maxConcurrentJobs)
	if err != nil {
		return err
	}
	p.removeTarget(name)
	p.targets[name] = target
	return nil
}

// DelTargets deletes/stops the removed targets from the configuration
func (p *HTTPGet) DelTargets() {
	p.logger.Debug("Current Targets", "type", "HTTPGet", "func", "DelTargets", "count", len(p.targets), "configured", countTargets(p.sc, "HTTPGet"))

	targetActiveTmp := []string{}
	for _, v := range p.targets {
		if v != nil {
			targetActiveTmp = common.AppendIfMissing(targetActiveTmp, v.Name())
		}
	}

	targetConfigTmp := []string{}
	for _, v := range p.sc.Cfg.Targets {
		if v.Type == "HTTPGet" {
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
func (p *HTTPGet) RemoveTarget(key string) {
	p.logger.Info("Removing Target", "type", "HTTPGet", "func", "RemoveTarget", "target", key)
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

// Snapshot returns a consistent point-in-time view of every monitored target
// (results, labels, and the full live name set) captured under a single read
// lock. This replaces the former ExportMetrics/ExportLabels/TargetNames trio,
// which each took the lock and walked the target map separately: that both
// re-locked every target several times per scrape and could skew the three
// views against each other under a concurrent add/remove.
func (p *HTTPGet) Snapshot() common.Snapshot[http.HTTPReturn] {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	snap := common.Snapshot[http.HTTPReturn]{
		Metrics: make(map[string]*http.HTTPReturn, len(p.targets)),
		Labels:  make(map[string]map[string]string, len(p.targets)),
		Names:   make([]string, 0, len(p.targets)),
	}
	for _, target := range p.targets {
		name := target.Name()
		snap.Names = append(snap.Names, name)
		if labels := target.Labels(); labels != nil {
			snap.Labels[name] = labels
		}
		if metrics := target.Compute(); metrics != nil {
			snap.Metrics[name] = metrics
		}
	}
	return snap
}
