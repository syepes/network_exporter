package target

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/syepes/network_exporter/pkg/http"
)

// HTTPGet Object
type HTTPGet struct {
	logger            *slog.Logger
	name              string
	url               string
	srcAddr           string
	proxy             string
	interval          time.Duration
	timeout           time.Duration
	maxConcurrentJobs int
	labels            map[string]string
	result            *http.HTTPReturn
	stop              chan struct{}
	wg                sync.WaitGroup
	sync.RWMutex
}

// NewHTTPGet starts a new monitoring goroutine
func NewHTTPGet(logger *slog.Logger, startupDelay time.Duration, name string, url string, srcAddr string, proxy string, interval time.Duration, timeout time.Duration, labels map[string]string, maxConcurrentJobs int) (*HTTPGet, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	t := &HTTPGet{
		logger:            logger,
		name:              name,
		url:               url,
		srcAddr:           srcAddr,
		proxy:             proxy,
		interval:          interval,
		timeout:           timeout,
		maxConcurrentJobs: maxConcurrentJobs,
		labels:            labels,
		stop:              make(chan struct{}),
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
			t.wg.Done()
			return
		}
	}

	waitChan := make(chan struct{}, t.maxConcurrentJobs)

	// Execute first probe immediately (after jitter delay)
	// This ensures targets start probing as quickly as possible
	select {
	case <-t.stop:
		t.wg.Done()
		return
	default:
		waitChan <- struct{}{}
		go func() {
			t.httpGetCheck()
			<-waitChan
		}()
	}

	tick := time.NewTicker(t.interval)
	defer tick.Stop()

	for {
		select {
		case <-t.stop:
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
			t.logger.Error("HTTP Get with proxy failed", "type", "HTTPGet", "func", "httpGetCheck", "err", err)
		}

	} else {
		data, err = http.HTTPGet(t.url, t.srcAddr, t.timeout)
		if err != nil {
			t.logger.Error("HTTP Get failed", "type", "HTTPGet", "func", "httpGetCheck", "err", err)
		}
	}

	bytes, err2 := json.Marshal(data)
	if err2 != nil {
		t.logger.Error("Failed to marshal result", "type", "HTTPGet", "func", "httpGetCheck", "err", err2)
	}
	t.logger.Debug("HTTP Get result", "type", "HTTPGet", "func", "httpGetCheck", "result", string(bytes))

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
