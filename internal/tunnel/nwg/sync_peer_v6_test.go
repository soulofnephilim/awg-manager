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

// Hostname с AAAA-only резолвом на работающем туннеле: в RCI — заглушка
// (NDMS такое имя не резолвит), в ядро — свежерезолвнутый v6, реестр
// стража обновлён (ключ, endpoint и spec для перерезолва DDNS).
func TestSyncPeer_HostnameResolvesV6UsesPlaceholderAndKernelSet(t *testing.T) {
	cs := newCaptureServer(t)
	op := newSyncTestOperator(t, cs.srv.URL)
	op.resolveFn = func(string) (string, int, error) { return "2a02:6b8::feed:ff", 51820, nil }
	op.guardRegister("awg20", guardEntry{iface: "nwg5", pubkey: "OLDKEY", endpoint: "[2a02::1]:1", spec: "old.example.com:1", name: "Wireguard5"})
	log := &eventLog{}
	calls := stubWGTool(t, log, "/opt/bin/wg", nil)

	stored := &storage.AWGTunnel{
		ID:       "awg20",
		NWGIndex: 5,
		Peer: storage.AWGPeer{
			PublicKey: "NEWKEY",
			Endpoint:  "vpn.example.com:51820",
		},
	}
	if err := op.SyncPeer(context.Background(), stored, "OLDKEY"); err != nil {
		t.Fatalf("SyncPeer: %v", err)
	}

	body := strings.Join(cs.bodies, "\n")
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
	entry, ok := op.guardGet("awg20")
	if !ok || entry.pubkey != "NEWKEY" || entry.endpoint != "[2a02:6b8::feed:ff]:51820" || entry.spec != "vpn.example.com:51820" {
		t.Fatalf("guard entry not updated: %+v", entry)
	}
}

// Смена endpoint'а работающего v6-туннеля на v4-литерал: реальный адрес
// уходит в RCI (им управляет NDMS), страж снимается — иначе он воскресил бы
// удалённого пира со старым ключом.
func TestSyncPeer_V6ToV4LiteralUnregistersGuard(t *testing.T) {
	cs := newCaptureServer(t)
	op := newSyncTestOperator(t, cs.srv.URL)
	op.guardRegister("awg20", guardEntry{iface: "nwg5", pubkey: "OLDKEY", endpoint: "[2a02::1]:51820", spec: "[2a02::1]:51820", name: "Wireguard5"})
	log := &eventLog{}
	calls := stubWGTool(t, log, "/opt/bin/wg", nil)

	stored := &storage.AWGTunnel{
		ID:       "awg20",
		NWGIndex: 5,
		Peer:     storage.AWGPeer{PublicKey: "NEWKEY", Endpoint: "1.2.3.4:51820"},
	}
	if err := op.SyncPeer(context.Background(), stored, "OLDKEY"); err != nil {
		t.Fatalf("SyncPeer: %v", err)
	}

	if !strings.Contains(strings.Join(cs.bodies, "\n"), "1.2.3.4:51820") {
		t.Fatalf("real v4 endpoint must reach RCI:\n%s", strings.Join(cs.bodies, "\n"))
	}
	if len(*calls) != 0 {
		t.Fatalf("wg set must not run for v4 endpoint: %v", *calls)
	}
	if op.guardHas("awg20") {
		t.Fatal("guard must be unregistered after switch to v4")
	}
}

// Резолв hostname'а упал, а параметры пира изменились: устаревший реестр
// стража снимается (wg set по старому ключу воскресил бы удалённого пира).
// Hostname уходит в RCI как раньше.
func TestSyncPeer_ResolveFailedChangedPeerDropsStaleGuard(t *testing.T) {
	cs := newCaptureServer(t)
	op := newSyncTestOperator(t, cs.srv.URL)
	op.resolveFn = func(string) (string, int, error) { return "", 0, context.DeadlineExceeded }
	op.guardRegister("awg20", guardEntry{iface: "nwg5", pubkey: "OLDKEY", endpoint: "[2a02::1]:51820", spec: "old.example.com:51820", name: "Wireguard5"})
	log := &eventLog{}
	calls := stubWGTool(t, log, "/opt/bin/wg", nil)

	stored := &storage.AWGTunnel{
		ID:       "awg20",
		NWGIndex: 5,
		Peer:     storage.AWGPeer{PublicKey: "NEWKEY", Endpoint: "vpn.example.com:51820"},
	}
	if err := op.SyncPeer(context.Background(), stored, "OLDKEY"); err != nil {
		t.Fatalf("SyncPeer: %v", err)
	}

	if !strings.Contains(strings.Join(cs.bodies, "\n"), "vpn.example.com:51820") {
		t.Fatalf("hostname must reach RCI unchanged on resolve failure:\n%s", strings.Join(cs.bodies, "\n"))
	}
	if len(*calls) != 0 {
		t.Fatalf("wg set must not run on resolve failure: %v", *calls)
	}
	if op.guardHas("awg20") {
		t.Fatal("stale guard entry must be unregistered when peer changed and resolve failed")
	}
}

// Резолв упал, но пир НЕ менялся (правка keepalive и т.п.): реестр стража
// не трогаем — транзиентный сбой DNS не должен снимать защиту endpoint'а.
func TestSyncPeer_ResolveFailedUnchangedPeerKeepsGuard(t *testing.T) {
	cs := newCaptureServer(t)
	op := newSyncTestOperator(t, cs.srv.URL)
	op.resolveFn = func(string) (string, int, error) { return "", 0, context.DeadlineExceeded }
	op.guardRegister("awg20", guardEntry{iface: "nwg5", pubkey: "KEY", endpoint: "[2a02::1]:51820", spec: "vpn.example.com:51820", name: "Wireguard5"})
	log := &eventLog{}
	_ = stubWGTool(t, log, "/opt/bin/wg", nil)

	stored := &storage.AWGTunnel{
		ID:       "awg20",
		NWGIndex: 5,
		Peer:     storage.AWGPeer{PublicKey: "KEY", Endpoint: "vpn.example.com:51820", PersistentKeepalive: 25},
	}
	if err := op.SyncPeer(context.Background(), stored, ""); err != nil {
		t.Fatalf("SyncPeer: %v", err)
	}
	if !op.guardHas("awg20") {
		t.Fatal("guard must survive transient resolve failure when peer unchanged")
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
