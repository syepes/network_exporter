package main

import (
	"context"
	"expvar"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/felixge/fgprof"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/syepes/network_exporter/collector"
	"github.com/syepes/network_exporter/config"
	"github.com/syepes/network_exporter/monitor"
	"github.com/syepes/network_exporter/pkg/common"
)

const version string = "1.8.9"

var (
	WebListenAddresses = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests").Default(":9427").Strings()
	WebSystemdSocket   = kingpin.Flag("web.system.socket", "WebSystemdSocket").Default("0").Bool()
	enableIpv6         = kingpin.Flag("ipv6", "ipv6 Enable").Default("true").Bool()
	WebMetricPath      = kingpin.Flag("web.metrics.path", "metric path").Default("/metrics").String()
	WebConfigFile      = kingpin.Flag("web.config.file", "Path to the web configuration file").Default("").String()
	configFile         = kingpin.Flag("config.file", "Exporter configuration file").Default("/app/cfg/network_exporter.yml").String()
	configFileHeaders  = HTTPHeader(kingpin.Flag("config.file.header", "Headers for loading configuration file from URL"))
	enableProfileing   = kingpin.Flag("profiling", "Enable Profiling (pprof + fgprof)").Default("false").Bool()
	// SCALING: maxConcurrentJobs controls how many probe operations can run concurrently per target.
	// Higher values increase throughput but consume more resources (memory, CPU, file descriptors).
	// Default: 3 operations per target
	// Recommended ranges based on target count:
	//   - Small deployments (<100 targets): 3-5
	//   - Medium deployments (100-1000 targets): 2-3
	//   - Large deployments (>1000 targets): 1-2
	maxConcurrentJobs = kingpin.Flag("max-concurrent-jobs", "Maximum concurrent probe operations per target (affects memory and CPU usage)").Default("3").Int()
	sc                = &config.SafeConfig{Cfg: &config.Config{}}
	logger            *slog.Logger
	// SCALING: icmpID is a shared counter across all PING and MTR targets (see pkg/common/type.go for limits)
	icmpID         *common.IcmpID
	monitorPING    *monitor.PING
	monitorMTR     *monitor.MTR
	monitorTCP     *monitor.TCPPort
	monitorHTTPGet *monitor.HTTPGet

	indexHTML = `<!doctype html><html><head> <meta charset="UTF-8"><title>Network Exporter (Version ` + version + `)</title></head><body><h1>Network Exporter</h1><p><a href="%s">Metrics</a></p></body></html>`
)

type HTTPHeaderValue http.Header

func (h *HTTPHeaderValue) Set(input string) error {
	name, value, found := strings.Cut(input, "=")
	if !found {
		return fmt.Errorf("expected HEADER=VALUE got '%s'", input)
	}
	(*http.Header)(h).Add(name, value)
	return nil
}

func (h *HTTPHeaderValue) String() string {
	return ""
}
func HTTPHeader(s kingpin.Settings) (target *http.Header) {
	target = &http.Header{}
	s.SetValue((*HTTPHeaderValue)(target))
	return
}

func init() {
	promslogConfig := &promslog.Config{}
	flag.AddFlags(kingpin.CommandLine, promslogConfig)
	kingpin.Version(version)
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger = promslog.New(promslogConfig)
	icmpID = &common.IcmpID{}
}

func main() {
	logger.Info("msg", "Starting network_exporter", "version", version)

	logger.Info("msg", "Loading config")
	if err := sc.ReloadConfig(logger, *configFile, *configFileHeaders); err != nil {
		logger.Error("msg", "Loading config", "err", err)
		os.Exit(1)
	}

	reloadSignal()

	resolver := getResolver()

	monitorPING = monitor.NewPing(logger, sc, resolver, icmpID, *enableIpv6, *maxConcurrentJobs)
	go monitorPING.AddTargets()

	monitorMTR = monitor.NewMTR(logger, sc, resolver, icmpID, *enableIpv6, *maxConcurrentJobs)
	go monitorMTR.AddTargets()

	monitorTCP = monitor.NewTCPPort(logger, sc, resolver, *enableIpv6, *maxConcurrentJobs)
	go monitorTCP.AddTargets()

	monitorHTTPGet = monitor.NewHTTPGet(logger, sc, resolver, *maxConcurrentJobs)
	go monitorHTTPGet.AddTargets()

	go startConfigRefresh()

	startServer()
}

func startConfigRefresh() {
	interval := sc.Cfg.Conf.Refresh.Duration()
	if interval <= 0 {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		logger.Info("msg", "ReLoading config")
		if err := sc.ReloadConfig(logger, *configFile, *configFileHeaders); err != nil {
			logger.Error("msg", "Reloading config skipped", "err", err)
			continue
		}
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
		logger.Info("msg", "Profiling enabled")
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

	logger.Info("msg", "Starting network_exporter", "version", version)
	logger.Info("msg", fmt.Sprintf("Listening for %s on %s", webMetricsPath, *WebListenAddresses))

	serverFlags := web.FlagConfig{
		WebConfigFile:      WebConfigFile,
		WebSystemdSocket:   WebSystemdSocket,
		WebListenAddresses: WebListenAddresses,
	}
	if err := web.ListenAndServe(server, &serverFlags, logger); err != nil {
		logger.Error("msg", "Could not start HTTP server", "err", err)
	}
}

func getResolver() *config.Resolver {
	if sc.Cfg.Conf.Nameserver == "" {
		logger.Info("msg", "Configured default DNS resolver")
		return &config.Resolver{Resolver: net.DefaultResolver, Timeout: sc.Cfg.Conf.NameserverTimeout.Duration()}
	}

	logger.Info("msg", "Configured custom DNS resolver")
	dialer := func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{Timeout: sc.Cfg.Conf.NameserverTimeout.Duration()}
		return d.DialContext(ctx, network, sc.Cfg.Conf.Nameserver)
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
