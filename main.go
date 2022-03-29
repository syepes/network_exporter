package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	_ "net/http/pprof"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/syepes/network_exporter/collector"
	"github.com/syepes/network_exporter/config"
	"github.com/syepes/network_exporter/monitor"
	"github.com/syepes/network_exporter/pkg/common"
	"gopkg.in/alecthomas/kingpin.v2"
)

const version string = "1.5.2"

var (
	listenAddress  = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests").Default(":9427").String()
	configFile     = kingpin.Flag("config.file", "Exporter configuration file").Default("/network_exporter.yml").String()
	sc             = &config.SafeConfig{Cfg: &config.Config{}}
	logger         log.Logger
	icmpID         *common.IcmpID // goroutine shared counter
	monitorPING    *monitor.PING
	monitorMTR     *monitor.MTR
	monitorTCP     *monitor.TCPPort
	monitorHTTPGet *monitor.HTTPGet

	indexHTML = `<!doctype html><html><head> <meta charset="UTF-8"><title>Network Exporter (Version ` + version + `)</title></head><body><h1>Network Exporter</h1><p><a href="%s">Metrics</a></p></body></html>`
)

func init() {
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version)
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger = promlog.New(promlogConfig)
	icmpID = &common.IcmpID{}
}

func main() {
	level.Info(logger).Log("msg", "Starting network_exporter", "version", version)

	level.Info(logger).Log("msg", "Loading config")
	if err := sc.ReloadConfig(logger, *configFile); err != nil {
		level.Error(logger).Log("msg", "Loading config", "err", err)
		os.Exit(1)
	}

	reloadSignal()

	resolver := getResolver()

	monitorPING = monitor.NewPing(logger, sc, resolver, icmpID)
	monitorPING.AddTargets()

	monitorMTR = monitor.NewMTR(logger, sc, resolver, icmpID)
	monitorMTR.AddTargets()

	monitorTCP = monitor.NewTCPPort(logger, sc, resolver)
	monitorTCP.AddTargets()

	monitorHTTPGet = monitor.NewHTTPGet(logger, sc, resolver)
	monitorHTTPGet.AddTargets()

	go startConfigRefresh()

	startServer()
}

func startConfigRefresh() {
	interval := sc.Cfg.Conf.Refresh.Duration()
	if interval <= 0 {
		return
	}

	for range time.NewTicker(interval).C {
		level.Info(logger).Log("msg", "ReLoading config")
		if err := sc.ReloadConfig(logger, *configFile); err != nil {
			level.Error(logger).Log("msg", "Reloading config skipped", "err", err)
			continue
		} else {
			monitorPING.DelTargets()
			monitorPING.CheckActiveTargets()
			monitorPING.AddTargets()
			monitorMTR.DelTargets()
			monitorMTR.CheckActiveTargets()
			monitorMTR.AddTargets()
			monitorTCP.DelTargets()
			monitorTCP.CheckActiveTargets()
			monitorTCP.AddTargets()
			monitorHTTPGet.DelTargets()
			monitorHTTPGet.AddTargets()
		}
	}
}

func startServer() {
	metricsPath := "/metrics"
	level.Info(logger).Log("msg", "Starting ping exporter", "version", version)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, indexHTML, metricsPath)
	})

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	reg.MustRegister(&collector.MTR{Monitor: monitorMTR})
	reg.MustRegister(&collector.PING{Monitor: monitorPING})
	reg.MustRegister(&collector.TCP{Monitor: monitorTCP})
	reg.MustRegister(&collector.HTTPGet{Monitor: monitorHTTPGet})
	h := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	http.Handle(metricsPath, h)

	level.Info(logger).Log("msg", fmt.Sprintf("Listening for %s on %s", metricsPath, *listenAddress))
	level.Error(logger).Log("msg", "Could not start http", "err", http.ListenAndServe(*listenAddress, nil))
}

func getResolver() *net.Resolver {
	if sc.Cfg.Conf.Nameserver == "" {
		level.Info(logger).Log("msg", "Configured default DNS resolver")
		return net.DefaultResolver
	}

	level.Info(logger).Log("msg", "Configured custom DNS resolver")
	dialer := func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{Timeout: 3 * time.Second}
		return d.DialContext(ctx, "udp", sc.Cfg.Conf.Nameserver)
	}
	return &net.Resolver{PreferGo: true, Dial: dialer}
}
