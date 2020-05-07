package target

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/syepes/ping_exporter/pkg/http"
)

// HTTPGet Object
type HTTPGet struct {
	logger   log.Logger
	name     string
	url      string
	proxy    string
	interval time.Duration
	timeout  time.Duration
	result   *http.HTTPReturn
	stop     chan struct{}
	wg       sync.WaitGroup
	sync.RWMutex
}

// NewHTTPGet starts a new monitoring goroutine
func NewHTTPGet(logger log.Logger, startupDelay time.Duration, name string, url string, proxy string, interval time.Duration, timeout time.Duration) (*HTTPGet, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	t := &HTTPGet{
		logger:   logger,
		name:     name,
		url:      url,
		proxy:    proxy,
		interval: interval,
		timeout:  timeout,
		stop:     make(chan struct{}),
	}
	t.wg.Add(1)
	go t.run(startupDelay)
	return t, nil
}

func (t *HTTPGet) run(startupDelay time.Duration) {
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
			go t.httpGetCheck()
		}
	}
}

// Stop gracefully stops the monitoring
func (t *HTTPGet) Stop() {
	close(t.stop)
	t.wg.Wait()
}

func (t *HTTPGet) httpGetCheck() {
	var data *http.HTTPReturn
	var err error

	if t.proxy != "" {
		data, err = http.HTTPGetProxy(t.url, t.timeout, t.proxy)
		if err != nil {
			level.Error(t.logger).Log("type", "HTTPGet", "func", "httpGetCheck", "msg", fmt.Sprintf("%s", err))
			return
		}

	} else {
		data, err = http.HTTPGet(t.url, t.timeout)
		if err != nil {
			level.Error(t.logger).Log("type", "HTTPGet", "func", "httpGetCheck", "msg", fmt.Sprintf("%s", err))
			return
		}
	}

	// bytes, err2 := json.Marshal(data)
	// if err2 != nil {
	// 	level.Error(t.logger).Log("type", "HTTPGet", "func", "httpGetCheck", "msg", fmt.Sprintf("%s", err2))
	// }
	// level.Debug(t.logger).Log("type", "HTTPGet", "func", "httpGetCheck", "msg", fmt.Sprintf("%s", string(bytes)))

	t.Lock()
	t.result = data
	t.Unlock()
}

// Compute returns the results of the Ping metrics
func (t *HTTPGet) Compute() *http.HTTPReturn {
	t.RLock()
	defer t.RUnlock()

	if t.result == nil {
		return nil
	}
	return t.result
}

// Name returns name
func (t *HTTPGet) Name() string {
	t.RLock()
	defer t.RUnlock()
	return t.name
}

// URL returns host
func (t *HTTPGet) URL() string {
	t.RLock()
	defer t.RUnlock()
	return t.url
}
