package nwg

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hoaxisr/awg-manager/internal/logging"
	"github.com/hoaxisr/awg-manager/internal/ndms/command"
	"github.com/hoaxisr/awg-manager/internal/ndms/query"
	"github.com/hoaxisr/awg-manager/internal/ndms/transport"
	"github.com/hoaxisr/awg-manager/internal/storage"
)

// startFakePoster — минимальный Poster для command.Commands: SyncAddressMTU
// и SyncDNS в startNative идут через commands, а не через transport-стаб.
type startFakePoster struct{}

func (startFakePoster) Post(context.Context, any) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

type startNopPublisher struct{}

func (startNopPublisher) Publish(string, any) {}

func newStartTestCommands() *command.Commands {
	q := query.NewQueries(query.Deps{
		Getter: query.NewFakeGetter(),
		Logger: query.NopLogger(),
		IsOS5:  func() bool { return true },
	})
	poster := startFakePoster{}
	sc := command.NewSaveCoordinator(poster, startNopPublisher{}, 500*time.Millisecond, 5*time.Second, 0, nil)
	return command.NewCommands(command.Deps{Poster: poster, Save: sc, Queries: q, IsOS5: func() bool { return true }})
}

// stubWGTool подменяет lookup/run wg на время теста.
func stubWGTool(t *testing.T, path string, run func(ctx context.Context, binary string, args ...string) error) *[][]string {
	t.Helper()
	origLookup, origRun := wgToolLookup, wgToolRun
	var calls [][]string
	wgToolLookup = func() string { return path }
	wgToolRun = func(ctx context.Context, binary string, args ...string) error {
		calls = append(calls, append([]string{binary}, args...))
		if run != nil {
			return run(ctx, binary, args...)
		}
		return nil
	}
	t.Cleanup(func() { wgToolLookup, wgToolRun = origLookup, origRun })
	return &calls
}

func newStartTestOperator(t *testing.T, srvURL string, resolvedIP string, port int) *OperatorNativeWG {
	t.Helper()
	return &OperatorNativeWG{
		transport:   transport.NewWithURL(srvURL, transport.NewSemaphore(2)),
		commands:    newStartTestCommands(),
		appLog:      logging.NewScopedLogger(nil, logging.GroupTunnel, logging.SubOps),
		supportsASC: func() bool { return true },
		resolveFn: func(endpoint string) (string, int, error) {
			return resolvedIP, port, nil
		},
	}
}

func startTestTunnel(endpoint string) *storage.AWGTunnel {
	return &storage.AWGTunnel{
		ID:       "awg20",
		Name:     "t-v6",
		NWGIndex: 3,
		Interface: storage.AWGInterface{
			PrivateKey:     "priv",
			Address:        "10.0.0.2/32",
			AWGObfuscation: storage.AWGObfuscation{Jc: 4, Jmin: 40, Jmax: 70},
		},
		Peer: storage.AWGPeer{
			PublicKey: "PUBKEY",
			Endpoint:  endpoint,
		},
	}
}

// v6: RCI-батч НЕ содержит endpoint-команду, endpoint уходит в ядро через
// `wg set nwg3 peer PUBKEY endpoint [v6]:port` ПОСЛЕ батча с up.
func TestStartNative_IPv6EndpointViaWGTool(t *testing.T) {
	s := newRCIStubServer(t, `[{}, {}]`)
	op := newStartTestOperator(t, s.srv.URL, "2a02:6b8::feed:ff", 51820)
	calls := stubWGTool(t, "/opt/bin/wg", nil)

	if err := op.startNative(context.Background(), startTestTunnel("[2a02:6b8::feed:ff]:51820")); err != nil {
		t.Fatalf("startNative: %v", err)
	}

	if strings.Contains(s.lastBody, "2a02:6b8") {
		t.Fatalf("v6 endpoint must not reach RCI:\n%s", s.lastBody)
	}
	if len(*calls) != 1 {
		t.Fatalf("wg tool calls = %d, want 1: %v", len(*calls), *calls)
	}
	got := strings.Join((*calls)[0], " ")
	want := "/opt/bin/wg set nwg3 peer PUBKEY endpoint [2a02:6b8::feed:ff]:51820"
	if got != want {
		t.Fatalf("wg call = %q, want %q", got, want)
	}
}

// v4: прежний путь байт-в-байт — endpoint в RCI-батче, wg не вызывается.
func TestStartNative_IPv4EndpointViaRCI(t *testing.T) {
	s := newRCIStubServer(t, `[{}, {}, {}]`)
	op := newStartTestOperator(t, s.srv.URL, "203.0.113.7", 51820)
	calls := stubWGTool(t, "/opt/bin/wg", nil)

	if err := op.startNative(context.Background(), startTestTunnel("203.0.113.7:51820")); err != nil {
		t.Fatalf("startNative: %v", err)
	}
	if !strings.Contains(s.lastBody, "203.0.113.7:51820") {
		t.Fatalf("v4 endpoint must go through RCI:\n%s", s.lastBody)
	}
	if len(*calls) != 0 {
		t.Fatalf("wg tool must not be called for v4: %v", *calls)
	}
}

// Нет бинаря wg — понятная ошибка ДО RCI-команд (интерфейс не дёргаем).
func TestStartNative_IPv6WithoutWGToolFailsFast(t *testing.T) {
	s := newRCIStubServer(t, `[{}, {}]`)
	op := newStartTestOperator(t, s.srv.URL, "2a02:6b8::1", 51820)
	stubWGTool(t, "", nil)

	err := op.startNative(context.Background(), startTestTunnel("[2a02:6b8::1]:51820"))
	if err == nil || !strings.Contains(err.Error(), "wireguard-tools") {
		t.Fatalf("want wireguard-tools hint error, got %v", err)
	}
	if s.lastBody != "" {
		t.Fatalf("RCI must not be touched when wg is missing, got body:\n%s", s.lastBody)
	}
}

// Ошибка wg set пробрасывается как ошибка старта.
func TestStartNative_WGToolFailurePropagates(t *testing.T) {
	s := newRCIStubServer(t, `[{}, {}]`)
	op := newStartTestOperator(t, s.srv.URL, "2a02:6b8::1", 51820)
	stubWGTool(t, "/opt/bin/wg", func(context.Context, string, ...string) error {
		return context.DeadlineExceeded
	})

	err := op.startNative(context.Background(), startTestTunnel("[2a02:6b8::1]:51820"))
	if err == nil || !strings.Contains(err.Error(), "wg set") {
		t.Fatalf("want wg set error, got %v", err)
	}
}
