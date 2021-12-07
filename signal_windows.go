//go:build windows
// +build windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/go-kit/log/level"
)

func reloadSignal() {

	// Signal handling
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
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
					monitorHTTPGet.AddTargets()
					monitorHTTPGet.DelTargets()
				}
			}
		}
	}()
}
