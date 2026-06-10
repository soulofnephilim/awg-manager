package monitoring

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hoaxisr/awg-manager/internal/icmpprobe"
	"github.com/hoaxisr/awg-manager/internal/sys/httpclient"
)

type stubDoer struct {
	result *httpclient.Result
	err    error
}

func (s stubDoer) Do(_ context.Context, _ httpclient.CallConfig) (*httpclient.Result, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

// HTTPProber latency = (time_connect - time_namelookup) * 1000 ms.
// TimeConnect and TimeNameLookup are cumulative-from-start (matching curl
// semantics), so the subtraction yields pure TCP RTT.
func TestHTTPProber_ParseLatency(t *testing.T) {
	cases := []struct {
		name   string
		result *httpclient.Result
		err    error
		wantOK bool
		wantMs int
	}{
		{
			name: "ok 200, TCP RTT 12ms",
			result: &httpclient.Result{
				Metrics: httpclient.Metrics{HTTPCode: 200, TimeNameLookup: 0.001, TimeConnect: 0.013, TimeTotal: 0.020},
			},
			wantOK: true,
			wantMs: 12,
		},
		{
			name: "ok with 404 still reachable, TCP RTT 25ms",
			result: &httpclient.Result{
				Metrics: httpclient.Metrics{HTTPCode: 404, TimeNameLookup: 0.002, TimeConnect: 0.027, TimeTotal: 0.030},
			},
			wantOK: true,
			wantMs: 25,
		},
		{
			name: "no response — code 0",
			result: &httpclient.Result{
				Metrics: httpclient.Metrics{HTTPCode: 0, TimeNameLookup: 0, TimeConnect: 0, TimeTotal: 5.0},
			},
			wantOK: false,
		},
		{
			name: "DNS slow — cumulative timings preserve correct RTT",
			result: &httpclient.Result{
				Metrics: httpclient.Metrics{HTTPCode: 200, TimeNameLookup: 0.200, TimeConnect: 0.280, TimeTotal: 0.390},
			},
			wantOK: true,
			wantMs: 80,
		},
		{
			name: "fallback to time_total when timings invalid",
			result: &httpclient.Result{
				Metrics: httpclient.Metrics{HTTPCode: 200, TimeNameLookup: 0.020, TimeConnect: 0.010, TimeTotal: 0.030},
			},
			wantOK: true,
			wantMs: 30,
		},
		{
			name:   "exec error",
			err:    errors.New("boom"),
			wantOK: false,
		},
		{
			name:   "garbage output (nil result)",
			result: nil,
			wantOK: false,
		},
		{
			name: "non-numeric code (treated as 0)",
			result: &httpclient.Result{
				Metrics: httpclient.Metrics{HTTPCode: 0, TimeNameLookup: 0.001, TimeConnect: 0.013, TimeTotal: 0.020},
			},
			wantOK: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &HTTPProber{Doer: stubDoer{result: c.result, err: c.err}}
			ms, ok := p.Probe(context.Background(), "1.1.1.1", "wg0", 5*time.Second)
			if ok != c.wantOK {
				t.Errorf("ok = %v, want %v", ok, c.wantOK)
			}
			if c.wantOK && ms != c.wantMs {
				t.Errorf("latency = %d, want %d", ms, c.wantMs)
			}
		})
	}
}

// ICMPProber maps icmpprobe results to (latency, ok).
func TestICMPProber_Probe(t *testing.T) {
	cases := []struct {
		name   string
		res    icmpprobe.Result
		err    error
		wantOK bool
		wantMs int
	}{
		{name: "success", res: icmpprobe.Result{LatencyMs: 14}, wantOK: true, wantMs: 14},
		{name: "probe error means failure", err: errors.New("no reply"), wantOK: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &ICMPProber{Pinger: func(context.Context, string, string, []string) (icmpprobe.Result, error) {
				return c.res, c.err
			}}
			ms, ok := p.Probe(context.Background(), "1.1.1.1", "wg0", 5*time.Second)
			if ok != c.wantOK {
				t.Errorf("ok = %v, want %v", ok, c.wantOK)
			}
			if c.wantOK && ms != c.wantMs {
				t.Errorf("latency = %d, want %d", ms, c.wantMs)
			}
		})
	}
}
