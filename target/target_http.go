package target

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/syepes/network_exporter/pkg/http"
)

// HTTPGet Object
type HTTPGet struct {
	logger   log.Logger
	name     string
	url      string
	srcAddr  string
	proxy    string
	interval time.Duration
	timeout  time.Duration
	labels   map[string]string
	result   *http.HTTPReturn
	stop     chan struct{}
	wg       sync.WaitGroup
	sync.RWMutex
}

// NewHTTPGet starts a new monitoring goroutine
func NewHTTPGet(logger log.Logger, startupDelay time.Duration, name string, url string, srcAddr string, proxy string, interval time.Duration, timeout time.Duration, labels map[string]string) (*HTTPGet, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	t := &HTTPGet{
		logger:   logger,
		name:     name,
		url:      url,
		srcAddr:  srcAddr,
		proxy:    proxy,
		interval: interval,
		timeout:  timeout,
		labels:   labels,
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

	waitChan := make(chan struct{}, MaxConcurrentJobs)
	tick := time.NewTicker(t.interval)
	for {
		select {
		case <-t.stop:
			tick.Stop()
			t.wg.Done()
			return
		case <-tick.C:
			waitChan <- struct{}{}
			go func() {
				t.httpGetCheck()
				<-waitChan
			}()
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
		}

	} else {
		data, err = http.HTTPGet(t.url, t.srcAddr, t.timeout)
		if err != nil {
			level.Error(t.logger).Log("type", "HTTPGet", "func", "httpGetCheck", "msg", fmt.Sprintf("%s", err))
		}
	}

	bytes, err2 := json.Marshal(data)
	if err2 != nil {
		level.Error(t.logger).Log("type", "HTTPGet", "func", "httpGetCheck", "msg", fmt.Sprintf("%s", err2))
	}
	level.Debug(t.logger).Log("type", "HTTPGet", "func", "httpGetCheck", "msg", bytes)

	t.Lock()
	defer t.Unlock()
	t.result = data
}

// Compute returns the results of the HTTP metrics
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

// Labels returns labels
func (t *HTTPGet) Labels() map[string]string {
	t.RLock()
	defer t.RUnlock()
	return t.labels
}
