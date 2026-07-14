package nwg

import (
	"context"
	"strings"
	"testing"

	"github.com/hoaxisr/awg-manager/internal/storage"
)

// SyncPeer с v6-endpoint'ом: в RCI уходит заглушка (NDMS отвергает v6 в
// peer-командах), реальный endpoint доезжает до ядра wg set'ом и реестр
// стража обновляется — для работающего туннеля.
func TestSyncPeer_IPv6EndpointUsesPlaceholderAndKernelSet(t *testing.T) {
	cs := newCaptureServer(t)
	op := newSyncTestOperator(t, cs.srv.URL)
	op.guardRegister("awg20", guardEntry{iface: "nwg5", pubkey: "OLDKEY", endpoint: "[2a02::1]:1", name: "Wireguard5"})
	log := &eventLog{}
	calls := stubWGTool(t, log, "/opt/bin/wg", nil)

	stored := &storage.AWGTunnel{
		ID:       "awg20",
		NWGIndex: 5,
		Peer: storage.AWGPeer{
			PublicKey: "NEWKEY",
			Endpoint:  "[2a02:6b8::feed:ff]:51820",
		},
	}
	if err := op.SyncPeer(context.Background(), stored, "OLDKEY"); err != nil {
		t.Fatalf("SyncPeer: %v", err)
	}

	body := strings.Join(cs.bodies, "\n")
	if strings.Contains(body, "2a02:6b8") {
		t.Fatalf("v6 endpoint must not reach RCI:\n%s", body)
	}
	if !strings.Contains(body, ndmsEndpointPlaceholder) {
		t.Fatalf("placeholder endpoint missing in RCI batch:\n%s", body)
	}
	if len(*calls) != 1 {
		t.Fatalf("wg set calls = %d, want 1: %v", len(*calls), *calls)
	}
	got := strings.Join((*calls)[0], " ")
	if got != "/opt/bin/wg set nwg5 peer NEWKEY endpoint [2a02:6b8::feed:ff]:51820" {
		t.Fatalf("unexpected wg set: %q", got)
	}

	// Реестр стража обновлён новым ключом и endpoint'ом.
	op.guardMu.Lock()
	entry := op.guard["awg20"]
	op.guardMu.Unlock()
	if entry.pubkey != "NEWKEY" || entry.endpoint != "[2a02:6b8::feed:ff]:51820" {
		t.Fatalf("guard entry not updated: %+v", entry)
	}
}

// Остановленный v6-туннель (не в страже): только заглушка в RCI, wg не
// вызывается — endpoint доедет при старте.
func TestSyncPeer_IPv6StoppedTunnelSkipsKernelSet(t *testing.T) {
	cs := newCaptureServer(t)
	op := newSyncTestOperator(t, cs.srv.URL)
	log := &eventLog{}
	calls := stubWGTool(t, log, "/opt/bin/wg", nil)

	stored := &storage.AWGTunnel{
		ID:       "awg21",
		NWGIndex: 6,
		Peer:     storage.AWGPeer{PublicKey: "K", Endpoint: "[2a02::1]:51820"},
	}
	if err := op.SyncPeer(context.Background(), stored, ""); err != nil {
		t.Fatalf("SyncPeer: %v", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("wg set must not run for a stopped tunnel: %v", *calls)
	}
}
