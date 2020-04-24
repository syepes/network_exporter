package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "net/http/pprof"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/syepes/ping_exporter/collector"
	"github.com/syepes/ping_exporter/config"
	"github.com/syepes/ping_exporter/monitor"
	"gopkg.in/alecthomas/kingpin.v2"
)

const version string = "1.1.1"

var (
	listenAddress = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests").Default(":9427").String()
	configFile    = kingpin.Flag("config.file", "Exporter configuration file").Default("/ping_exporter.yml").String()
	sc            = &config.SafeConfig{Cfg: &config.Config{}}
	logger        log.Logger
	monitorPING   *monitor.PING
	monitorMTR    *monitor.MTR
	monitorTCP    *monitor.TCPPort

	indexHTML = `<!doctype html><html><head> <meta charset="UTF-8"><title>Ping Exporter (Version ` + version + `)</title></head><body><h1>Ping Exporter</h1><p><a href="%s">Metrics</a></p></body></html>`
)

func init() {
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version)
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger = promlog.New(promlogConfig)
}

func main() {
	level.Info(logger).Log("msg", "Starting ping_exporter", "version", version)

	level.Info(logger).Log("msg", "Loading config")
	if err := sc.ReloadConfig(*configFile); err != nil {
		level.Error(logger).Log("msg", "Loading config", "err", err)
		os.Exit(1)
	}

	// Signal handling
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	susr := make(chan os.Signal, 1)
	signal.Notify(susr, syscall.SIGUSR1)
	go func() {
		for {
			select {
			case <-hup:
				level.Debug(logger).Log("msg", "Signal: HUP")
				level.Info(logger).Log("msg", "ReLoading config")
				if err := sc.ReloadConfig(*configFile); err != nil {
					level.Error(logger).Log("msg", "Reloading config skipped", "err", err)
					continue
				} else {
					monitorPING.AddTargets()
					monitorPING.DelTargets()
					monitorMTR.AddTargets()
					monitorMTR.DelTargets()
					monitorTCP.AddTargets()
					monitorTCP.DelTargets()
				}
			case <-susr:
				level.Debug(logger).Log("msg", "Signal: USR1")
			}
		}
	}()

	resolver := getResolver()

	monitorPING = monitor.NewPing(logger, sc, resolver)
	monitorPING.AddTargets()

	monitorMTR = monitor.NewMTR(logger, sc, resolver)
	monitorMTR.AddTargets()

	monitorTCP = monitor.NewTCPPort(logger, sc, resolver)
	monitorTCP.AddTargets()

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
		if err := sc.ReloadConfig(*configFile); err != nil {
			level.Error(logger).Log("msg", "Reloading config skipped", "err", err)
			continue
		} else {
			monitorPING.AddTargets()
			monitorPING.DelTargets()
			monitorMTR.AddTargets()
			monitorMTR.DelTargets()
			monitorTCP.AddTargets()
			monitorTCP.DelTargets()
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
	reg.MustRegister(prometheus.NewGoCollector())
	reg.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	reg.MustRegister(&collector.MTR{Monitor: monitorMTR})
	reg.MustRegister(&collector.PING{Monitor: monitorPING})
	reg.MustRegister(&collector.TCP{Monitor: monitorTCP})
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
