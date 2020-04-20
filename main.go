package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

const version string = "0.6.0"

var (
	listenAddress = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests").Default(":9427").String()
	configFile    = kingpin.Flag("config.file", "Exporter configuration file").Default("/ping_exporter.yml").String()
	sc            = &config.SafeConfig{Cfg: &config.Config{}}
	logger        log.Logger
	monitorICMP   *monitor.MonitorPing

	indexHTML = `<!doctype html><html><head> <meta charset="UTF-8"><title>Ping Exporter (Version ` + version + `)</title></head><body><h1>Ping Exporter</h1><p><a href="%s">Metrics</a></p></body></html>`
)

// var targets = []string{"109.6.13.29", "109.6.13.29"}
var targets = []string{"109.6.13.29"}

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
					level.Error(logger).Log("msg", "Reloading config", "err", err)
					continue
				} else {
					addTargets(logger)
					delTargets(logger)
				}
			case <-susr:
				level.Debug(logger).Log("msg", "Signal: USR1")
				// refreshDNS(logger)
			}
		}
	}()

	monitorICMP = monitor.New(logger, 6*time.Second, 1*time.Second, 3)
	// monitorICMP.AddTarget("google test", "109.6.13.29")
	addTargets(logger)

	startServer(logger)

	// // MTR
	// for _, val := range targets {
	// 	go func(target string) {
	// 		for {
	// 			mm, err := mtr.Mtr(target, 30, 4, 300)
	// 			if err != nil {
	// 				fmt.Println(err)
	// 			}
	// 			// fmt.Println(mm)

	// 			bytes, err2 := json.Marshal(mm)
	// 			if err2 != nil {
	// 				fmt.Println(err2)
	// 			}
	// 			fmt.Println("MTR:",string(bytes))

	// 			time.Sleep(10 * time.Second)
	// 		}
	// 	}(val)
	// }

	// PING
	// for _, val := range targets {
	// 	go func(target string) {
	// 		for {
	// 			// mm, err := ping.Ping(target, 4, 3*time.Second, 10*time.Millisecond)
	// 			mm, err := ping.Ping(target, 4, 3*time.Second, 10*time.Millisecond)
	// 			if err != nil {
	// 				fmt.Println(err)
	// 			}

	// 			bytes, err2 := json.Marshal(mm)
	// 			if err2 != nil {
	// 				fmt.Println(err2)
	// 			}
	// 			fmt.Println("PING:", string(bytes))

	// 			time.Sleep(10 * time.Second)
	// 		}
	// 	}(val)
	// }
	// select {}

}

func startServer(logger log.Logger) {
	metricsPath := "/metrics"
	level.Info(logger).Log("msg", "Starting ping exporter", "version", version)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, indexHTML, metricsPath)
	})

	reg := prometheus.NewRegistry()
	// reg.MustRegister(prometheus.NewGoCollector())
	// reg.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	reg.MustRegister(&collector.PingCollector{Monitor: monitorICMP})
	// reg.MustRegister(&pingCollector{monitor: monitorICMP})
	// reg.MustRegister(&mtrCollector{monitor: monitorMTR})
	h := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	http.Handle(metricsPath, h)

	level.Info(logger).Log("msg", fmt.Sprintf("Listening for %s on %s", metricsPath, *listenAddress))
	level.Error(logger).Log("msg", "Could not start http", "err", http.ListenAndServe(*listenAddress, nil))
}

func addTargets(logger log.Logger) {
	targetsICMP := monitorICMP.TargetList()
	level.Debug(logger).Log("msg", fmt.Sprintf("targetsICMP: %d, cfg: %d", len(targetsICMP), len(sc.Cfg.Dest)), "func", "addTargets")

	targetActiveTmp := []string{}
	for _, v := range targetsICMP {
		targetActiveTmp = appendIfMissing(targetActiveTmp, v.ID())
	}

	targetConfigTmp := []string{}
	for _, v := range sc.Cfg.Dest {
		targetConfigTmp = appendIfMissing(targetConfigTmp, v.Alias+"::"+v.Host)
	}

	targetAdd := compareList(targetActiveTmp, targetConfigTmp)
	level.Info(logger).Log("msg", fmt.Sprintf("targetID: %v", targetAdd))

	for _, targetID := range targetAdd {
		for _, host := range sc.Cfg.Dest {
			if host.Alias+"::"+host.Host != targetID {
				continue
			}

			if host.Type == "ICMP" {
				monitorICMP.AddTarget(host.Alias, host.Host)
			} else if host.Type == "MTR" {

			} else {

			}
		}
	}
}

func delTargets(logger log.Logger) {
	targetsICMP := monitorICMP.TargetList()
	level.Debug(logger).Log("msg", fmt.Sprintf("targetsICMP: %d, cfg: %d", len(targetsICMP), len(sc.Cfg.Dest)), "func", "delTargets")

	targetActiveTmp := []string{}
	for _, v := range targetsICMP {
		if v != nil {
			targetActiveTmp = appendIfMissing(targetActiveTmp, v.ID())
		}
	}

	targetConfigTmp := []string{}
	for _, v := range sc.Cfg.Dest {
		targetConfigTmp = appendIfMissing(targetConfigTmp, v.Alias+"::"+v.Host)
	}

	targetDelete := compareList(targetConfigTmp, targetActiveTmp)
	for _, targetID := range targetDelete {
		for _, t := range targetsICMP {
			if t == nil {
				continue
			}
			if t.ID() == targetID {
				monitorICMP.RemoveTarget(targetID)
			}
		}
	}
}

func compareList(a, b []string) []string {
	var tmpList []string
	ma := make(map[string]bool, len(a))
	for _, ka := range a {
		ma[ka] = true
	}
	for _, kb := range b {
		if !ma[kb] {
			tmpList = append(tmpList, kb)
		}
	}
	return tmpList
}

func appendIfMissing(slice []string, i string) []string {
	for _, v := range slice {
		if v == i {
			return slice
		}
	}
	return append(slice, i)
}

func isIPAddrInSlice(ipa net.IPAddr, slice []net.IPAddr) bool {
	for _, x := range slice {
		if x.IP.Equal(ipa.IP) {
			return true
		}
	}
	return false
}
