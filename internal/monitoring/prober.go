package monitoring

import (
	"context"
	"net/http"
	"time"

	"github.com/hoaxisr/awg-manager/internal/icmpprobe"
	"github.com/hoaxisr/awg-manager/internal/sys/httpclient"
)

// Prober probes a single host through a specific interface and returns
// latency in milliseconds + success flag. Implementations must be safe for
// concurrent use.
type Prober interface {
	Probe(ctx context.Context, host, ifaceName string, timeout time.Duration) (latencyMs int, ok bool)
}

// HTTPDoer is the minimal surface needed by HTTPProber.
type HTTPDoer interface {
	Do(ctx context.Context, cfg httpclient.CallConfig) (*httpclient.Result, error)
}

// HTTPProber probes via HTTPS HEAD through a Go-native HTTP client with
// SO_BINDTODEVICE and reports the **TCP RTT** — `time_connect - time_namelookup`.
// This matches the metric reported by the per-tunnel connectivity-check service
// so numbers in the monitoring matrix line up.
//
// "Reachable" is defined as: any HTTP status code (>0) before the timeout.
// 4xx/5xx still counts — TCP+TLS handshake completed through the tunnel.
type HTTPProber struct {
	Doer HTTPDoer
}

// NewHTTPProber builds a prober backed by the package-level httpclient.
func NewHTTPProber() *HTTPProber {
	return &HTTPProber{Doer: httpclient.DefaultClient}
}

// Probe issues a single HTTPS HEAD request through ifaceName.
// ok=false on context cancellation, client error, or http_code == 0 (no response).
func (p *HTTPProber) Probe(ctx context.Context, host, ifaceName string, timeout time.Duration) (int, bool) {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout+1*time.Second)
	defer cancel()

	res, err := p.Doer.Do(timeoutCtx, httpclient.CallConfig{
		URL:            "https://" + host + "/",
		Method:         http.MethodHead,
		Interface:      ifaceName,
		ConnectTimeout: 3 * time.Second,
		MaxTime:        timeout,
		DiscardBody:    true,
	})
	if err != nil || res == nil {
		return 0, false
	}

	if res.Metrics.HTTPCode == 0 {
		return 0, false
	}

	// Prefer pure TCP RTT — DNS resolution can dominate time_total on first
	// requests after a tunnel comes up. Fall back to time_total when the
	// per-phase timings look bogus.
	var latencyMs int
	if res.Metrics.TimeConnect > 0 && res.Metrics.TimeConnect >= res.Metrics.TimeNameLookup {
		latencyMs = int((res.Metrics.TimeConnect - res.Metrics.TimeNameLookup) * 1000)
	} else {
		latencyMs = int(res.Metrics.TimeTotal * 1000)
	}
	if latencyMs <= 0 {
		latencyMs = 1
	}
	return latencyMs, true
}

// ICMPProber sends a single native ICMP echo bound to the tunnel
// interface. Used for matrix cells whose target is the tunnel's
// connectivity-check self host AND the tunnel's method is "ping".
type ICMPProber struct {
	Pinger func(ctx context.Context, ifaceName, target string, dnsServers []string) (icmpprobe.Result, error)
}

// NewICMPProber builds an ICMP prober backed by the native icmpprobe.
func NewICMPProber() *ICMPProber {
	return &ICMPProber{Pinger: icmpprobe.ByInterface}
}

// Probe sends a single ICMP echo. ok=false on resolve/socket/timeout error.
func (p *ICMPProber) Probe(ctx context.Context, host, ifaceName string, timeout time.Duration) (int, bool) {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := p.Pinger(timeoutCtx, ifaceName, host, nil)
	if err != nil {
		return 0, false
	}
	return res.LatencyMs, true
}
