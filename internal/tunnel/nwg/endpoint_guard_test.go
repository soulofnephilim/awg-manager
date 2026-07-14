package nwg

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hoaxisr/awg-manager/internal/logging"
)

// TestMain отодвигает тикер guardLoop за горизонт любого прогона: каждый
// guardRegister через guardOnce запускает вечный цикл, и с дефолтными 20с
// утёкший цикл раннего теста мог бы выполнить sweep поверх стабов позже
// идущего теста (фантомные wg-вызовы, гонка с восстановлением глобалов).
func TestMain(m *testing.M) {
	guardInterval = time.Hour
	os.Exit(m.Run())
}

func newGuardTestOperator() *OperatorNativeWG {
	return &OperatorNativeWG{
		appLog: logging.NewScopedLogger(nil, logging.GroupTunnel, logging.SubOps),
	}
}

// stubGuardLookup подменяет полный резолв имени в sweep; счётчик вызовов
// возвращается для ассертов.
func stubGuardLookup(t *testing.T, ips []string, err error) *int {
	t.Helper()
	orig := guardLookupIPs
	calls := new(int)
	guardLookupIPs = func(string) ([]string, error) {
		*calls++
		return ips, err
	}
	t.Cleanup(func() { guardLookupIPs = orig })
	return calls
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

// DDNS-имя стало резолвиться в новый адрес (старый выпал из резолва):
// страж обновляет ожидание в реестре и доводит ядро до нового адреса,
// даже если старый endpoint в ядре «на месте».
func TestGuardSweep_ReresolvesDDNSAndUpdatesKernel(t *testing.T) {
	op := newGuardTestOperator()
	_ = stubGuardLookup(t, []string{"2a02::feed"}, nil)
	op.guardRegister("awg20", guardEntry{iface: "nwg3", pubkey: "PUB", endpoint: "[2a02::1]:51820", spec: "vpn.example.com:51820", name: "Wireguard3"})
	calls := stubGuardWG(t, "PUB\t[2a02::1]:51820\n", nil)

	op.guardSweep(context.Background())

	if len(*calls) != 1 {
		t.Fatalf("wg set calls = %d, want 1: %v", len(*calls), *calls)
	}
	got := strings.Join((*calls)[0], " ")
	if got != "/opt/bin/wg set nwg3 peer PUB endpoint [2a02::feed]:51820" {
		t.Fatalf("unexpected wg set: %q", got)
	}
	entry, _ := op.guardGet("awg20")
	if entry.endpoint != "[2a02::feed]:51820" {
		t.Fatalf("guard entry endpoint not updated: %+v", entry)
	}
}

// Анти-флап: round-robin DNS отдал записи в другом порядке, но текущий
// адрес всё ещё среди них — endpoint не трогаем, живая сессия не дёргается.
func TestGuardSweep_RoundRobinRotationNoFlap(t *testing.T) {
	op := newGuardTestOperator()
	_ = stubGuardLookup(t, []string{"2a02::2", "2a02::1"}, nil)
	op.guardRegister("awg20", guardEntry{iface: "nwg3", pubkey: "PUB", endpoint: "[2a02::1]:51820", spec: "vpn.example.com:51820", name: "Wireguard3"})
	calls := stubGuardWG(t, "PUB\t[2a02::1]:51820\n", nil)

	op.guardSweep(context.Background())

	if len(*calls) != 0 {
		t.Fatalf("rotation must not flap endpoint, got %v", *calls)
	}
	entry, _ := op.guardGet("awg20")
	if entry.endpoint != "[2a02::1]:51820" {
		t.Fatalf("guard entry must be unchanged: %+v", entry)
	}
}

// Dual-stack после переезда: текущий v6 выпал из резолва, у имени остались
// A+AAAA — выбирается v4 (то же предпочтение, что у netutil.ResolveHost).
func TestGuardSweep_DualStackPrefersV4WhenCurrentGone(t *testing.T) {
	op := newGuardTestOperator()
	_ = stubGuardLookup(t, []string{"2a02::2", "198.51.100.7"}, nil)
	op.guardRegister("awg20", guardEntry{iface: "nwg3", pubkey: "PUB", endpoint: "[2a02::1]:51820", spec: "vpn.example.com:51820", name: "Wireguard3"})
	calls := stubGuardWG(t, "PUB\t[2a02::1]:51820\n", nil)

	op.guardSweep(context.Background())

	if len(*calls) != 1 {
		t.Fatalf("wg set calls = %d, want 1: %v", len(*calls), *calls)
	}
	got := strings.Join((*calls)[0], " ")
	if got != "/opt/bin/wg set nwg3 peer PUB endpoint 198.51.100.7:51820" {
		t.Fatalf("unexpected wg set: %q", got)
	}
}

// DNS недоступен: страж работает по последнему известному адресу —
// восстановление слетевшего endpoint'а не блокируется сбоем резолва.
func TestGuardSweep_ResolveFailureFallsBackToLastKnown(t *testing.T) {
	op := newGuardTestOperator()
	_ = stubGuardLookup(t, nil, context.DeadlineExceeded)
	op.guardRegister("awg20", guardEntry{iface: "nwg3", pubkey: "PUB", endpoint: "[2a02::1]:51820", spec: "vpn.example.com:51820", name: "Wireguard3"})
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

// Литеральный spec не перерезолвливается — резолвер не дёргается вовсе.
func TestGuardSweep_LiteralSpecSkipsResolve(t *testing.T) {
	op := newGuardTestOperator()
	lookups := stubGuardLookup(t, nil, context.DeadlineExceeded)
	op.guardRegister("awg20", guardEntry{iface: "nwg3", pubkey: "PUB", endpoint: "[2a02::1]:51820", spec: "[2a02::1]:51820", name: "Wireguard3"})
	_ = stubGuardWG(t, "PUB\t[2a02::1]:51820\n", nil)

	op.guardSweep(context.Background())

	if *lookups != 0 {
		t.Fatalf("resolver must not run for literal spec, ran %d times", *lookups)
	}
}

// Запись заменена/удалена, пока sweep читал wg show: перепроверка перед
// wg set не даёт установить endpoint по устаревшему снапшоту (wg set по
// отсутствующему ключу воскресил бы удалённого пира).
func TestGuardSweep_RecheckBeforeSetSkipsReplacedEntry(t *testing.T) {
	op := newGuardTestOperator()
	op.guardRegister("awg20", guardEntry{iface: "nwg3", pubkey: "OLDKEY", endpoint: "[2a02::1]:51820", spec: "[2a02::1]:51820", name: "Wireguard3"})

	origLookup, origRun, origOut := wgToolLookup, wgToolRun, wgToolOutput
	var calls [][]string
	wgToolLookup = func() string { return "/opt/bin/wg" }
	wgToolOutput = func(context.Context, string, ...string) (string, error) {
		// Пока sweep «читал» wg show, параллельный SyncPeer заменил пира.
		op.guardReplaceIfPresent("awg20", guardEntry{iface: "nwg3", pubkey: "NEWKEY", endpoint: "[2a02::2]:51820", spec: "[2a02::2]:51820", name: "Wireguard3"})
		return "NEWKEY\t[2a02::2]:51820\n", nil
	}
	wgToolRun = func(_ context.Context, binary string, args ...string) error {
		calls = append(calls, append([]string{binary}, args...))
		return nil
	}
	t.Cleanup(func() { wgToolLookup, wgToolRun, wgToolOutput = origLookup, origRun, origOut })

	op.guardSweep(context.Background())

	if len(calls) != 0 {
		t.Fatalf("stale snapshot must not wg set, got %v", calls)
	}
}

// guardUpdateEndpoint: не воскрешает удалённую запись и не затирает
// заменённую (другой spec) резолвом старого имени.
func TestGuardUpdateEndpoint_StaleTargetsIgnored(t *testing.T) {
	op := newGuardTestOperator()

	op.guardRegister("awg20", guardEntry{iface: "nwg3", pubkey: "PUB", endpoint: "[2a02::1]:51820", spec: "b.example.com:51820", name: "Wireguard3"})
	op.guardUpdateEndpoint("awg20", "a.example.com:51820", "[2a02::feed]:51820")
	if entry, _ := op.guardGet("awg20"); entry.endpoint != "[2a02::1]:51820" {
		t.Fatalf("update with stale spec must be ignored: %+v", entry)
	}

	op.guardUnregister("awg20")
	op.guardUpdateEndpoint("awg20", "b.example.com:51820", "[2a02::feed]:51820")
	if op.guardHas("awg20") {
		t.Fatal("update must not resurrect an unregistered entry")
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
