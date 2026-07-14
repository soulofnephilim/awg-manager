package nwg

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/hoaxisr/awg-manager/internal/logging"
)

func newGuardTestOperator() *OperatorNativeWG {
	return &OperatorNativeWG{
		appLog: logging.NewScopedLogger(nil, logging.GroupTunnel, logging.SubOps),
	}
}

// stubGuardWG подменяет wg show/set: show отдаёт заданный вывод, set пишется
// в calls.
func stubGuardWG(t *testing.T, showOut string, showErr error) *[][]string {
	t.Helper()
	origLookup, origRun, origOut := wgToolLookup, wgToolRun, wgToolOutput
	var mu sync.Mutex
	var calls [][]string
	wgToolLookup = func() string { return "/opt/bin/wg" }
	wgToolOutput = func(context.Context, string, ...string) (string, error) {
		return showOut, showErr
	}
	wgToolRun = func(_ context.Context, binary string, args ...string) error {
		mu.Lock()
		calls = append(calls, append([]string{binary}, args...))
		mu.Unlock()
		return nil
	}
	t.Cleanup(func() { wgToolLookup, wgToolRun, wgToolOutput = origLookup, origRun, origOut })
	return &calls
}

// Endpoint слетел (NDMS переприменил конфиг — в ядре заглушка) → страж
// возвращает его на место.
func TestGuardSweep_RestoresDriftedEndpoint(t *testing.T) {
	op := newGuardTestOperator()
	op.guardRegister("awg20", guardEntry{iface: "nwg3", pubkey: "PUB", endpoint: "[2a02::1]:51820", name: "Wireguard3"})
	calls := stubGuardWG(t, "PUB\t127.0.0.1:1\n", nil)

	op.guardSweep(context.Background())

	if len(*calls) != 1 {
		t.Fatalf("wg set calls = %d, want 1: %v", len(*calls), *calls)
	}
	got := strings.Join((*calls)[0], " ")
	if got != "/opt/bin/wg set nwg3 peer PUB endpoint [2a02::1]:51820" {
		t.Fatalf("unexpected wg set: %q", got)
	}
}

// Endpoint на месте → страж молчит.
func TestGuardSweep_NoopWhenEndpointMatches(t *testing.T) {
	op := newGuardTestOperator()
	op.guardRegister("awg20", guardEntry{iface: "nwg3", pubkey: "PUB", endpoint: "[2a02::1]:51820", name: "Wireguard3"})
	calls := stubGuardWG(t, "PUB\t[2a02::1]:51820\n", nil)

	op.guardSweep(context.Background())

	if len(*calls) != 0 {
		t.Fatalf("no wg set expected, got %v", *calls)
	}
}

// После unregister (Stop/Delete) страж туннель не трогает.
func TestGuardSweep_UnregisteredTunnelIgnored(t *testing.T) {
	op := newGuardTestOperator()
	op.guardRegister("awg20", guardEntry{iface: "nwg3", pubkey: "PUB", endpoint: "[2a02::1]:51820", name: "Wireguard3"})
	op.guardUnregister("awg20")
	calls := stubGuardWG(t, "PUB\t127.0.0.1:1\n", nil)

	op.guardSweep(context.Background())

	if len(*calls) != 0 {
		t.Fatalf("no wg set expected after unregister, got %v", *calls)
	}
	if op.guardHas("awg20") {
		t.Fatal("guardHas must be false after unregister")
	}
}

func TestWGShowHasEndpoint(t *testing.T) {
	out := "OTHER\t1.2.3.4:51820\nPUB\t[2a02::1]:51820\n"
	if !wgShowHasEndpoint(out, "PUB", "[2a02::1]:51820") {
		t.Fatal("must find matching endpoint")
	}
	if wgShowHasEndpoint(out, "PUB", "[2a02::2]:51820") {
		t.Fatal("mismatched endpoint must be reported")
	}
	if wgShowHasEndpoint(out, "MISSING", "[2a02::1]:51820") {
		t.Fatal("missing peer must be reported as mismatch")
	}
}
