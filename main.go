package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/syepes/ping_exporter/monitor/ping"
	"gopkg.in/alecthomas/kingpin.v2"
)

const version string = "0.6.0"

var (
	listenAddress = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests").Default(":9427").String()
	indexHTML     = `<!doctype html><html><head> <meta charset="UTF-8"><title>Ping Exporter (Version ` + version + `)</title></head><body><h1>Ping Exporter</h1><p><a href="%s">Metrics</a></p></body></html>`
)

// var targets = []string{"109.6.13.29", "109.6.13.29"}
var targets = []string{"109.6.13.29"}

func main() {
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
	for _, val := range targets {
		go func(target string) {
			for {
				// mm, err := ping.Ping(target, 4, 3*time.Second, 10*time.Millisecond)
				mm, err := ping.Ping(target, 4, 3*time.Second, 10*time.Millisecond)
				if err != nil {
					fmt.Println(err)
				}

				bytes, err2 := json.Marshal(mm)
				if err2 != nil {
					fmt.Println(err2)
				}
				fmt.Println("PING:", string(bytes))

				time.Sleep(10 * time.Second)
			}
		}(val)
	}
	select {}

	// promlogConfig := &promlog.Config{}
	// flag.AddFlags(kingpin.CommandLine, promlogConfig)
	// kingpin.Version(version)
	// kingpin.HelpFlag.Short('h')
	// kingpin.Parse()
	// logger := promlog.New(promlogConfig)
	// level.Info(logger).Log("msg", "Starting ping_exporter", "version", version)

	// startServer(logger)

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
	// reg.MustRegister(&pingCollector{monitor: monitorICMP})
	// reg.MustRegister(&mtrCollector{monitor: monitorMTR})
	h := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	http.Handle(metricsPath, h)

	level.Info(logger).Log("msg", fmt.Sprintf("Listening for %s on %s", metricsPath, *listenAddress))
	level.Error(logger).Log("msg", "Could not start http", "err", http.ListenAndServe(*listenAddress, nil))
}
