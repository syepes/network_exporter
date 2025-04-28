package monitor

import (
	"log/slog"
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
	logger     *slog.Logger
	sc         *config.SafeConfig
	resolver   *config.Resolver
	interval   time.Duration
	timeout    time.Duration
	targets    map[string]*target.HTTPGet
	mtx        sync.RWMutex
}

// NewHTTPGet creates and configures a new Monitoring HTTPGet instance
func NewHTTPGet(logger *slog.Logger, sc *config.SafeConfig, resolver *config.Resolver) *HTTPGet {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	return &HTTPGet{
		logger:     logger,
		sc:         sc,
		resolver:   resolver,
		interval:   sc.Cfg.HTTPGet.Interval.Duration(),
		timeout:    sc.Cfg.HTTPGet.Timeout.Duration(),
		targets:    make(map[string]*target.HTTPGet),
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
				if target.Proxy != "" {
					err := p.AddTarget(target.Name, target.Host, target.SourceIp, target.Proxy, target.Labels.Kv)
					if err != nil {
						p.logger.Warn("Skipping target", "type", "HTTPGet", "func", "AddTargets", "host", target.Host, "err", err)
					}
				} else {
					err := p.AddTarget(target.Name, target.Host, target.SourceIp, "", target.Labels.Kv)
					if err != nil {
						p.logger.Warn("Skipping target", "type", "HTTPGet", "func", "AddTargets", "host", target.Host, "err", err)
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

	target, err := target.NewHTTPGet(p.logger, startupDelay, name, dURL.String(), srcAddr, proxy, p.interval, p.timeout, labels)
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

// Export collects the metrics for each monitored target and returns it as a simple map
func (p *HTTPGet) ExportMetrics() map[string]*http.HTTPReturn {
	m := make(map[string]*http.HTTPReturn)

	p.mtx.RLock()
	defer p.mtx.RUnlock()

	for _, target := range p.targets {
		name := target.Name()
		metrics := target.Compute()

		if metrics != nil {
			// p.logger.Debug("Export metrics", "type", "HTTPGet", "func", "ExportMetrics", "name", name, "metrics", metrics, "labels", target.Labels())
			m[name] = metrics
		}
	}
	return m
}

// ExportLabels target labels
func (p *HTTPGet) ExportLabels() map[string]map[string]string {
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
