package http

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"time"
)

// HTTPGet Http Get Trace Operation
func HTTPGet(destURL string, srcAddr string, timeout time.Duration) (*HTTPReturn, error) {
	var out HTTPReturn
	var err error
	out.DestAddr = destURL

	dURL, err := url.Parse(destURL)
	if err != nil {
		out.Success = false
		return &out, err
	}

	client := &http.Client{
		Timeout: timeout,
	}

	if srcAddr != "" {
		srcIp := net.ParseIP(srcAddr)
		if srcIp == nil {
			out.Success = false
			return &out, fmt.Errorf("source ip: %v is invalid, HTTP target: %v", srcAddr, destURL)
		}
		transport := &http.Transport{
			Dial: (&net.Dialer{
				LocalAddr: &net.TCPAddr{
					IP:   srcIp,
					Port: 0,
				},
			}).Dial}

		client = &http.Client{
			Timeout:   timeout,
			Transport: transport,
		}
	}

	req, err := http.NewRequest("GET", dURL.String(), nil)
	if err != nil {
		out.Success = false
		return &out, err
	}

	trace, ht := NewClientTrace()
	ctx := httptrace.WithClientTrace(req.Context(), trace)
	req = req.WithContext(ctx)
	req.Close = true

	resp, err := client.Do(req)
	if err != nil {
		out.Success = false
		return &out, err
	}

	// Read
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
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
	out.TLSHandshake = stats.TLSHandshake
	if resp.TLS != nil {
		out.TLSVersion = getTLSVersion(resp.TLS)
		out.TLSEarliestCertExpiry = getEarliestCertExpiry(resp.TLS)
		out.TLSLastChainExpiry = getLastChainExpiry(resp.TLS)
	}
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
		Proxy: http.ProxyURL(pURL),
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	req, err := http.NewRequest("GET", dURL.String(), nil)
	if err != nil {
		out.Success = false
		return &out, err
	}

	trace, ht := NewClientTrace()
	ctx := httptrace.WithClientTrace(req.Context(), trace)
	req = req.WithContext(ctx)
	req.Close = true

	resp, err := client.Do(req)
	if err != nil {
		out.Success = false
		return &out, err
	}

	// Read
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
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
	out.TLSHandshake = stats.TLSHandshake
	if resp.TLS != nil {
		out.TLSVersion = getTLSVersion(resp.TLS)
		out.TLSEarliestCertExpiry = getEarliestCertExpiry(resp.TLS)
		out.TLSLastChainExpiry = getLastChainExpiry(resp.TLS)
	}
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

func getTLSVersion(state *tls.ConnectionState) string {
	switch state.Version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return "unknown"
	}
}

func getEarliestCertExpiry(state *tls.ConnectionState) time.Time {
	earliest := time.Time{}
	for _, cert := range state.PeerCertificates {
		if (earliest.IsZero() || cert.NotAfter.Before(earliest)) && !cert.NotAfter.IsZero() {
			earliest = cert.NotAfter
		}
	}
	return earliest
}

func getLastChainExpiry(state *tls.ConnectionState) time.Time {
	lastChainExpiry := time.Time{}
	for _, chain := range state.VerifiedChains {
		earliestCertExpiry := time.Time{}
		for _, cert := range chain {
			if (earliestCertExpiry.IsZero() || cert.NotAfter.Before(earliestCertExpiry)) && !cert.NotAfter.IsZero() {
				earliestCertExpiry = cert.NotAfter
			}
		}
		if lastChainExpiry.IsZero() || lastChainExpiry.Before(earliestCertExpiry) {
			lastChainExpiry = earliestCertExpiry
		}

	}
	return lastChainExpiry
}
