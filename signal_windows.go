//go:build windows
// +build windows

package main

// reloadSignal is a no-op on Windows: the OS has no SIGHUP/SIGUSR1 delivery
// mechanism, so signal-based configuration reload is not possible there. Use
// the HTTP endpoint (POST /-/reload) or a positive `refresh` config interval
// for on-demand configuration reload instead.
func reloadSignal() {
	logger.Info("Signal-based reload is unavailable on Windows; use HTTP 'POST /-/reload' or the 'refresh' config interval")
}
