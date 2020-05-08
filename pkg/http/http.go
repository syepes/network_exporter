package http

import (
	"crypto/tls"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"time"
)

// HTTPGet Http Get Trace Operation
func HTTPGet(destURL string, timeout time.Duration) (*HTTPReturn, error) {
	var out HTTPReturn
	var err error
	out.DestAddr = destURL

	dURL, err := url.Parse(destURL)
	if err != nil {
		out.Success = false
		return &out, err
	}

	// Control timeout
	transport := &http.Transport{
		Dial:                (&net.Dialer{Timeout: timeout}).Dial,
		TLSHandshakeTimeout: timeout,
	}
	client := &http.Client{Transport: transport}

	req, err := http.NewRequest("GET", dURL.String(), nil)
	if err != nil {
		out.Success = false
		return &out, err
	}

	trace, ht := NewClientTrace()
	ctx := httptrace.WithClientTrace(req.Context(), trace)
	req = req.WithContext(ctx)

	resp, err := client.Do(req)
	if err != nil {
		out.Success = false
		return &out, err
	}

	// Read
	if _, err := io.Copy(ioutil.Discard, resp.Body); err != nil {
		out.Success = false
		return &out, err
	}

	ht.Finish()
	stats := ht.Stats()

	out.Success = true
	out.Status = resp.StatusCode
	out.ContentLength = resp.ContentLength
	out.DNSLookup = stats.DNSLookup
	out.TCPConnection = stats.TCPConnection
	out.TLSHandshake = stats.TCPConnection
	out.ServerProcessing = stats.ServerProcessing
	out.ContentTransfer = stats.ContentTransfer
	out.Total = stats.Total

	return &out, nil
}

// HTTPGetProxy Http Get Trace Operation with proxy
func HTTPGetProxy(destURL string, timeout time.Duration, proxyURL string) (*HTTPReturn, error) {
	var out HTTPReturn
	var err error
	out.DestAddr = destURL

	dURL, err := url.Parse(destURL)
	if err != nil {
		out.Success = false
		return &out, err
	}

	pURL, err := url.Parse(proxyURL)
	if err != nil {
		out.Success = false
		return &out, err
	}

	// Control timeout and proxy
	transport := &http.Transport{
		Dial:                (&net.Dialer{Timeout: timeout}).Dial,
		TLSHandshakeTimeout: timeout,
		Proxy:               http.ProxyURL(pURL),
	}
	client := &http.Client{Transport: transport}

	req, err := http.NewRequest("GET", dURL.String(), nil)
	if err != nil {
		out.Success = false
		return &out, err
	}

	trace, ht := NewClientTrace()
	ctx := httptrace.WithClientTrace(req.Context(), trace)
	req = req.WithContext(ctx)

	resp, err := client.Do(req)
	if err != nil {
		out.Success = false
		return &out, err
	}

	// Read
	if _, err := io.Copy(ioutil.Discard, resp.Body); err != nil {
		out.Success = false
		return &out, err
	}

	ht.Finish()
	stats := ht.Stats()

	out.Success = true
	out.Status = resp.StatusCode
	out.ContentLength = resp.ContentLength
	out.DNSLookup = stats.DNSLookup
	out.TCPConnection = stats.TCPConnection
	out.TLSHandshake = stats.TCPConnection
	out.ServerProcessing = stats.ServerProcessing
	out.ContentTransfer = stats.ContentTransfer
	out.Total = stats.Total

	return &out, nil
}

// NewClientTrace http client trace
func NewClientTrace() (trace *httptrace.ClientTrace, ht *HTTPTrace) {
	ht = &HTTPTrace{
		Start: time.Now(),
		// will be false when connect start event
		TCPReused: true,
	}
	trace = &httptrace.ClientTrace{
		DNSStart: func(info httptrace.DNSStartInfo) {
			ht.Lock()
			defer ht.Unlock()
			ht.Host = info.Host
			ht.DNSStart = time.Now()
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			ht.Lock()
			defer ht.Unlock()
			ht.Addrs = make([]string, len(info.Addrs))
			for index, addr := range info.Addrs {
				ht.Addrs[index] = addr.String()
			}
			ht.DNSDone = time.Now()
		},
		ConnectStart: func(network, addr string) {
			ht.Lock()
			defer ht.Unlock()
			ht.TCPReused = false
			ht.Network = network
			ht.Addr = addr
			ht.ConnectStart = time.Now()
		},
		ConnectDone: func(_, _ string, _ error) {
			ht.Lock()
			defer ht.Unlock()
			ht.ConnectDone = time.Now()
		},
		GotConn: func(info httptrace.GotConnInfo) {
			ht.Lock()
			defer ht.Unlock()
			ht.Reused = info.Reused
			ht.WasIdle = info.WasIdle
			ht.IdleTime = info.IdleTime
			ht.GotConnect = time.Now()
		},
		TLSHandshakeStart: func() {
			ht.Lock()
			defer ht.Unlock()
			ht.TLSHandshakeStart = time.Now()
		},
		TLSHandshakeDone: func(info tls.ConnectionState, _ error) {
			ht.Lock()
			defer ht.Unlock()
			ht.TLSResume = info.DidResume
			ht.Protocol = info.NegotiatedProtocol
			ht.TLSHandshakeDone = time.Now()
		},
		GotFirstResponseByte: func() {
			ht.Lock()
			defer ht.Unlock()
			ht.GotFirstResponseByte = time.Now()
		},
	}

	return
}

// Finish http trace finish
func (ht *HTTPTrace) Finish() {
	ht.Lock()
	defer ht.Unlock()
	ht.Done = time.Now()
}

// Stats get the stats of time line
func (ht *HTTPTrace) Stats() (stats *HTTPTimelineStats) {
	stats = &HTTPTimelineStats{}
	ht.RLock()
	defer ht.RUnlock()

	// fmt.Printf("HTTPTrace: %+v\n", ht)

	if !ht.DNSStart.IsZero() && !ht.DNSDone.IsZero() {
		stats.DNSLookup = ht.DNSDone.Sub(ht.DNSStart)
	}
	if !ht.ConnectStart.IsZero() && !ht.ConnectDone.IsZero() {
		stats.TCPConnection = ht.ConnectDone.Sub(ht.ConnectStart)
	}
	if !ht.TLSHandshakeStart.IsZero() && !ht.TLSHandshakeDone.IsZero() {
		stats.TLSHandshake = ht.TLSHandshakeDone.Sub(ht.TLSHandshakeStart)
	}
	if !ht.GotConnect.IsZero() && !ht.GotFirstResponseByte.IsZero() {
		stats.ServerProcessing = ht.GotFirstResponseByte.Sub(ht.GotConnect)
	}
	if ht.Done.IsZero() {
		ht.Done = time.Now()
	}
	if !ht.GotFirstResponseByte.IsZero() {
		stats.ContentTransfer = ht.Done.Sub(ht.GotFirstResponseByte)
	}
	stats.Total = ht.Done.Sub(ht.Start)

	return
}
