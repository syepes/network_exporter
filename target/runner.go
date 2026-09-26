package target

import (
	"time"

	"github.com/syepes/network_exporter/pkg/selfmon"
)

// acquireSlot blocks until a concurrency slot on waitChan is free or the target
// is stopped, recording how long a scheduled probe of type typ waited for the
// slot via the internal queue-wait metric. It returns false if the target was
// stopped while waiting, in which case no slot was acquired and the caller must
// not run the probe.
//
// A non-blocking fast path avoids touching the clock when a slot is immediately
// available (the common, unsaturated case), so queue wait is only measured when
// the probe actually has to wait.
func acquireSlot(typ string, waitChan chan struct{}, stop <-chan struct{}) bool {
	select {
	case waitChan <- struct{}{}:
		selfmon.ObserveQueueWait(typ, 0)
		return true
	case <-stop:
		return false
	default:
	}

	start := time.Now()
	select {
	case waitChan <- struct{}{}:
		selfmon.ObserveQueueWait(typ, time.Since(start))
		return true
	case <-stop:
		return false
	}
}
