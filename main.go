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
	"sync"
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
	"github.com/syepes/network_exporter/pkg/selfmon"
)

const version string = "2.0.0"

var (
	WebListenAddresses = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests").Default(":9427").Strings()
	WebSystemdSocket   = kingpin.Flag("web.system.socket", "WebSystemdSocket").Default("0").Bool()
	enableIpv6         = kingpin.Flag("ipv6", "ipv6 Enable").Default("true").Bool()
	WebMetricPath      = kingpin.Flag("web.metrics.path", "metric path").Default("/metrics").String()
	WebConfigFile      = kingpin.Flag("web.config.file", "Path to the web configuration file").Default("").String()
	configFile         = kingpin.Flag("config.file", "Exporter configuration file").Default("/app/cfg/network_exporter.yml").String()
	configFileHeaders  = HTTPHeader(kingpin.Flag("config.file.header", "Headers for loading configuration file from URL"))
	enableProfileing   = kingpin.Flag("profiling", "Enable Profiling (pprof + fgprof)").Default("false").Bool()
	// SCALING: maxConcurrentJobs bounds how many probe cycles may overlap for a
	// SINGLE target. Overlap only happens when a probe takes longer than the
	// target's `interval` to complete; the per-target run loop then applies
	// back-pressure once this many probes are already in flight for that target.
	// This is a PER-TARGET limit, not a global one: with N targets the worst-case
	// global concurrency is N*maxConcurrentJobs, so it is not a total resource cap.
	// A global concurrency budget is planned as part of the scheduler rework.
	// Default: 3 overlapping probes per target.
	maxConcurrentJobs = kingpin.Flag("max-concurrent-jobs", "Maximum overlapping probe cycles per target (per-target, not a global cap)").Default("3").Int()
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
	logger.Info("Starting network_exporter", "version", version)

	logger.Info("Loading config")
	if err := sc.ReloadConfig(logger, *configFile, *configFileHeaders); err != nil {
		logger.Error("Loading config", "err", err)
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

// reloadMtx serializes configuration reloads so the multi-step
// Del/Check/Add sequence never races on the monitor target maps when
// triggered concurrently by the interval ticker, an OS signal, or the HTTP
// endpoint.
var reloadMtx sync.Mutex

// reloadConfig re-reads the configuration file and reconciles every monitor's
// targets. It is safe to call concurrently from any trigger; calls are
// serialized via reloadMtx. The trigger label identifies the reload source in
// the logs. On a config load error the previous configuration stays active.
func reloadConfig(trigger string) error {
	reloadMtx.Lock()
	defer reloadMtx.Unlock()

	logger.Info("ReLoading config", "trigger", trigger)
	if err := sc.ReloadConfig(logger, *configFile, *configFileHeaders); err != nil {
		logger.Error("Reloading config skipped", "trigger", trigger, "err", err)
		return err
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
	return nil
}

func startConfigRefresh() {
	interval := sc.Cfg.Conf.Refresh.Duration()
	if interval <= 0 {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		_ = reloadConfig("interval")
	}
}

func startServer() {
	mux := http.NewServeMux()
	webMetricsPath := *WebMetricPath

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	// Internal self-monitoring metrics for probe scheduling/execution health.
	selfmon.Register(reg)
	reg.MustRegister(&collector.MTR{Monitor: monitorMTR})
	reg.MustRegister(&collector.PING{Monitor: monitorPING})
	reg.MustRegister(&collector.TCP{Monitor: monitorTCP})
	reg.MustRegister(&collector.HTTPGet{Monitor: monitorHTTPGet})
	h := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	mux.Handle(webMetricsPath, h)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, indexHTML, webMetricsPath)
	})

	// Cross-platform on-demand configuration reload. This is the only on-demand
	// reload mechanism available on Windows (which has no SIGHUP delivery).
	// POST/PUT only so a stray GET or link prefetch cannot trigger a reload.
	mux.HandleFunc("/-/reload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodPut {
			w.Header().Set("Allow", "POST, PUT")
			http.Error(w, "This endpoint requires a POST or PUT request.", http.StatusMethodNotAllowed)
			return
		}
		if err := reloadConfig("HTTP"); err != nil {
			http.Error(w, fmt.Sprintf("Failed to reload config: %v", err), http.StatusInternalServerError)
			return
		}
		fmt.Fprintln(w, "Configuration reloaded successfully")
	})

	if *enableProfileing {
		logger.Info("Profiling enabled")
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

	logger.Info("Starting network_exporter", "version", version)
	logger.Info(fmt.Sprintf("Listening for %s on %s", webMetricsPath, *WebListenAddresses))

	serverFlags := web.FlagConfig{
		WebConfigFile:      WebConfigFile,
		WebSystemdSocket:   WebSystemdSocket,
		WebListenAddresses: WebListenAddresses,
	}
	if err := web.ListenAndServe(server, &serverFlags, logger); err != nil {
		logger.Error("Could not start HTTP server", "err", err)
	}
}

func getResolver() *config.Resolver {
	if sc.Cfg.Conf.Nameserver == "" {
		logger.Info("Configured default DNS resolver")
		return &config.Resolver{Resolver: net.DefaultResolver, Timeout: sc.Cfg.Conf.NameserverTimeout.Duration(), TTL: sc.Cfg.Conf.NameserverCacheTTL.Duration()}
	}

	logger.Info("Configured custom DNS resolver")
	dialer := func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{Timeout: sc.Cfg.Conf.NameserverTimeout.Duration()}
		return d.DialContext(ctx, network, sc.Cfg.Conf.Nameserver)
	}
	return &config.Resolver{Resolver: &net.Resolver{PreferGo: true, Dial: dialer}, Timeout: sc.Cfg.Conf.NameserverTimeout.Duration(), TTL: sc.Cfg.Conf.NameserverCacheTTL.Duration()}
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
