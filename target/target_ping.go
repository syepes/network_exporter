package target

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/syepes/ping_exporter/pkg/ping"
)

// PING Object
type PING struct {
	logger   log.Logger
	name     string
	host     string
	interval time.Duration
	timeout  time.Duration
	count    int
	result   *ping.PingReturn
	stop     chan struct{}
	wg       sync.WaitGroup
	sync.RWMutex
}

// NewPing starts a new monitoring goroutine
func NewPing(logger log.Logger, startupDelay time.Duration, name string, host string, interval time.Duration, timeout time.Duration, count int) (*PING, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	t := &PING{
		logger:   logger,
		name:     name,
		host:     host,
		timeout:  timeout,
		interval: interval,
		count:    count,
		stop:     make(chan struct{}),
	}
	t.wg.Add(1)
	go t.run(startupDelay)
	return t, nil
}

func (t *PING) run(startupDelay time.Duration) {
	if startupDelay > 0 {
		select {
		case <-time.After(startupDelay):
		case <-t.stop:
		}
	}

	tick := time.NewTicker(t.interval)
	for {
		select {
		case <-t.stop:
			tick.Stop()
			t.wg.Done()
			return
		case <-tick.C:
			go t.ping()
		}
	}
}

// Stop gracefully stops the monitoring
func (t *PING) Stop() {
	close(t.stop)
	t.wg.Wait()
}

func (t *PING) ping() {
	data, err := ping.Ping(t.host, t.count, t.interval, t.timeout)
	if err != nil {
		level.Error(t.logger).Log("type", "ICMP", "func", "ping", "msg", fmt.Sprintf("%s", err))
	}

	// bytes, err2 := json.Marshal(data)
	// if err2 != nil {
	// 	level.Error(t.logger).Log("type", "ICMP", "func", "ping", "msg", fmt.Sprintf("%s", err2))
	// }
	// level.Debug(t.logger).Log("type", "ICMP", "func", "ping", "msg", fmt.Sprintf("%s", string(bytes)))

	t.Lock()
	t.result = data
	t.Unlock()
}

// Compute returns the results of the Ping metrics
func (t *PING) Compute() *ping.PingReturn {
	t.RLock()
	defer t.RUnlock()

	if t.result == nil {
		return nil
	}
	return t.result
}

// Name returns name
func (t *PING) Name() string {
	t.RLock()
	defer t.RUnlock()
	return t.name
}

// Host returns host
func (t *PING) Host() string {
	t.RLock()
	defer t.RUnlock()
	return t.host
}
