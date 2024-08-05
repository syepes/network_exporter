package main

import (
	"context"
	"expvar"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/felixge/fgprof"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/syepes/network_exporter/collector"
	"github.com/syepes/network_exporter/config"
	"github.com/syepes/network_exporter/monitor"
	"github.com/syepes/network_exporter/pkg/common"
)

const version string = "1.7.8"

var (
	WebListenAddresses = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests").Default(":9427").Strings()
	WebSystemdSocket   = kingpin.Flag("web.system.socket", "WebSystemdSocket").Default("0").Bool()
	WebMetricPath      = kingpin.Flag("web.metrics.path", "metric path").Default("/metrics").String()
	WebConfigFile      = kingpin.Flag("web.config.file", "Path to the web configuration file").Default("").String()
	configFile         = kingpin.Flag("config.file", "Exporter configuration file").Default("/app/cfg/network_exporter.yml").String()
	enableProfileing   = kingpin.Flag("profiling", "Enable Profiling (pprof + fgprof)").Default("false").Bool()
	sc                 = &config.SafeConfig{Cfg: &config.Config{}}
	logger             log.Logger
	icmpID             *common.IcmpID // goroutine shared counter
	monitorPING        *monitor.PING
	monitorMTR         *monitor.MTR
	monitorTCP         *monitor.TCPPort
	monitorHTTPGet     *monitor.HTTPGet

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
	go monitorPING.AddTargets()

	monitorMTR = monitor.NewMTR(logger, sc, resolver, icmpID)
	go monitorMTR.AddTargets()

	monitorTCP = monitor.NewTCPPort(logger, sc, resolver)
	go monitorTCP.AddTargets()

	monitorHTTPGet = monitor.NewHTTPGet(logger, sc, resolver)
	go monitorHTTPGet.AddTargets()

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
			_ = monitorPING.CheckActiveTargets()
			monitorPING.AddTargets()
			monitorMTR.DelTargets()
			_ = monitorMTR.CheckActiveTargets()
			monitorMTR.AddTargets()
			monitorTCP.DelTargets()
			_ = monitorTCP.CheckActiveTargets()
			monitorTCP.AddTargets()
			monitorHTTPGet.DelTargets()
			monitorHTTPGet.AddTargets()
		}
	}
}

func startServer() {
	mux := http.NewServeMux()
	webMetricsPath := *WebMetricPath

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	reg.MustRegister(&collector.MTR{Monitor: monitorMTR})
	reg.MustRegister(&collector.PING{Monitor: monitorPING})
	reg.MustRegister(&collector.TCP{Monitor: monitorTCP})
	reg.MustRegister(&collector.HTTPGet{Monitor: monitorHTTPGet})
	h := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	mux.Handle(webMetricsPath, h)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, indexHTML, webMetricsPath)
	})

	if *enableProfileing {
		level.Info(logger).Log("msg", "Profiling enabled")
		mux.Handle("/debug/vars", http.HandlerFunc(expVars))
		mux.HandleFunc("/debug/fgprof", fgprof.Handler().(http.HandlerFunc))
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	server := &http.Server{
		Handler: mux,
	}

	level.Info(logger).Log("msg", "Starting network_exporter", "version", version)
	level.Info(logger).Log("msg", fmt.Sprintf("Listening for %s on %s", webMetricsPath, *WebListenAddresses))

	serverFlags := web.FlagConfig{
		WebConfigFile:      WebConfigFile,
		WebSystemdSocket:   WebSystemdSocket,
		WebListenAddresses: WebListenAddresses,
	}
	if err := web.ListenAndServe(server, &serverFlags, logger); err != nil {
		level.Error(logger).Log("msg", "Could not start HTTP server", "err", err)
	}
}

func getResolver() *config.Resolver {
	if sc.Cfg.Conf.Nameserver == "" {
		level.Info(logger).Log("msg", "Configured default DNS resolver")
		return &config.Resolver{Resolver: net.DefaultResolver, Timeout: sc.Cfg.Conf.NameserverTimeout.Duration()}
	}

	level.Info(logger).Log("msg", "Configured custom DNS resolver")
	dialer := func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{Timeout: sc.Cfg.Conf.NameserverTimeout.Duration()}
		return d.DialContext(ctx, "udp", sc.Cfg.Conf.Nameserver)
	}
	return &config.Resolver{Resolver: &net.Resolver{PreferGo: true, Dial: dialer}, Timeout: sc.Cfg.Conf.NameserverTimeout.Duration()}
}

func expVars(w http.ResponseWriter, r *http.Request) {
	first := true
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintf(w, "{\n")
	expvar.Do(func(kv expvar.KeyValue) {
		if !first {
			fmt.Fprintf(w, ",\n")
		}
		first = false
		fmt.Fprintf(w, "%q: %s", kv.Key, kv.Value)
	})
	fmt.Fprintf(w, "\n}\n")
}
