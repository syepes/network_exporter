package monitor

import (
	"strings"

	"github.com/syepes/network_exporter/config"
)

// countTargets Count the number of target by type
func countTargets(sc *config.SafeConfig, target string) (count int) {
	count = 0
	for _, v := range sc.Cfg.Targets {
		if strings.Contains(strings.ToUpper(v.Type), strings.ToUpper(target)) {
			count++
		}
	}
	return count
}
