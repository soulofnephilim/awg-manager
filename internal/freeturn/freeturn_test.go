package freeturn

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// link.go
// ---------------------------------------------------------------------------

func TestLink_Roundtrip(t *testing.T) {
	p := LinkPayload{V: 1, Provider: "vk", Peer: "1.2.3.4:56000", Obf: "rtpopus2", Key: "aabb", MTU: 1376, WG: "[Interface]\nPrivateKey = x\n"}
	link, err := EncodeLink(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link, LinkScheme) {
		t.Fatalf("no scheme prefix: %q", link)
	}
	if strings.HasSuffix(link, "=") {
		t.Fatalf("padding must be stripped (JS-generator parity): %q", link)
	}
	got, err := DecodeLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, p) {
		t.Fatalf("roundtrip mismatch:\n got %+v\nwant %+v", got, p)
	}
}

func TestDecodeLink_WithoutScheme(t *testing.T) {
	link, _ := EncodeLink(LinkPayload{V: 1, Peer: "h:1"})
	got, err := DecodeLink(strings.TrimPrefix(link, LinkScheme))
	if err != nil || got.Peer != "h:1" {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestDecodeLink_Rejects(t *testing.T) {
	for _, bad := range []string{"", "freeturn://", "freeturn://%%%", "freeturn://aGVsbG8"} {
		if _, err := DecodeLink(bad); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

// ---------------------------------------------------------------------------
// service.go — CLI-аргументы
// ---------------------------------------------------------------------------

func TestBuildClientArgs_FullAndZero(t *testing.T) {
	full := ClientConfig{
		Listen: "127.0.0.1:9000", Peer: "h:56000", Provider: "vk",
		Links: "https://vk.ru/call/join/a", Streams: 4, Transport: "tcp",
		Mode: "udp", Bond: true, TurnHost: "turn.host", TurnPort: 3478,
		ObfProfile: "rtpopus2", ObfKey: "deadbeef", StreamsPerCred: 2,
		Browser: "chromium", ManualCaptcha: true, DNSMode: "doh",
		DNSServers: "1.1.1.1", ClientID: "cid", Sub: "s", Debug: true,
	}
	want := []string{
		"-listen", "127.0.0.1:9000", "-peer", "h:56000", "-provider", "vk",
		"-links", "https://vk.ru/call/join/a", "-n", "4", "-transport", "tcp",
		"-mode", "udp", "-bond", "-turn", "turn.host", "-port", "3478",
		"-obf-profile", "rtpopus2", "-obf-key", "deadbeef",
		"-streams-per-cred", "2", "-browser", "chromium", "-manual-captcha",
		"-dns-mode", "doh", "-dns-servers", "1.1.1.1", "-client-id", "cid",
		"-sub", "s", "-debug",
	}
	if got := buildClientArgs(full); !reflect.DeepEqual(got, want) {
		t.Fatalf("full args:\n got %v\nwant %v", got, want)
	}
	// Нулевые значения не эмитятся — остаются дефолты бинаря.
	if got := buildClientArgs(ClientConfig{}); len(got) != 0 {
		t.Fatalf("zero config must emit no args, got %v", got)
	}
}

func TestBuildServerArgs(t *testing.T) {
	got := buildServerArgs(ServerConfig{Listen: "0.0.0.0:56000", Connect: "127.0.0.1:51820", ObfProfile: "rtpopus", Debug: true})
	want := []string{"-listen", "0.0.0.0:56000", "-connect", "127.0.0.1:51820", "-obf-profile", "rtpopus", "-debug"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// store.go
// ---------------------------------------------------------------------------

func TestStore_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	cfg, err := s.Load() // отсутствующий файл → дефолты
	if err != nil {
		t.Fatal(err)
	}
	cfg.Client.Peer = "h:56000"
	cfg.Server.Connect = "127.0.0.1:51820"
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := NewStore(dir).Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Client.Peer != "h:56000" || got.Server.Connect != "127.0.0.1:51820" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}

// ---------------------------------------------------------------------------
// process.go — через seam startCmd; p.binary указывает на /bin/sh, чтобы
// пройти проверку binaryPresent, а реальная команда подменяется seam'ом.
// ---------------------------------------------------------------------------

func newTestProcess(t *testing.T, script string) *process {
	t.Helper()
	p := newProcess("client", "/bin/sh", t.TempDir())
	p.startCmd = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("/bin/sh", "-c", script)
	}
	return p
}

func TestProcess_StartupFailureCapturesStderr(t *testing.T) {
	p := newTestProcess(t, "echo boom >&2; exit 1")
	err := p.Start(nil)
	if err == nil {
		t.Fatal("want startup error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("stderr tail not in error: %v", err)
	}
	if running, _ := p.IsRunning(); running {
		t.Fatal("must not be running after startup failure")
	}
	if st := p.Status(); st.LastError == "" {
		t.Fatal("LastError must survive for the status endpoint")
	}
}

func TestProcess_StartStop(t *testing.T) {
	p := newTestProcess(t, "sleep 30")
	if err := p.Start(nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if running, _ := p.IsRunning(); !running {
		t.Fatal("must be running after grace period")
	}
	if st := p.Status(); !st.Running || st.PID == 0 || st.StartedAt == nil {
		t.Fatalf("bad status: %+v", st)
	}
	if err := p.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if running, _ := p.IsRunning(); !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("still running after Stop")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Штатная остановка — не ошибка: лог не должен попадать в LastError.
	if st := p.Status(); st.LastError != "" {
		t.Fatalf("clean Stop must not leave LastError, got %q", st.LastError)
	}
}

func TestProcess_StartMissingBinary(t *testing.T) {
	p := newProcess("client", "/nonexistent/freeturn-client", t.TempDir())
	err := p.Start(nil)
	if err == nil || !strings.Contains(err.Error(), "не найден") {
		t.Fatalf("want clear missing-binary error, got %v", err)
	}
}

func TestBinaryPresent(t *testing.T) {
	if binaryPresent("/nonexistent/path") {
		t.Error("missing path must be absent")
	}
	if !binaryPresent("/bin/sh") {
		t.Error("/bin/sh must be present+executable")
	}
	if binaryPresent(t.TempDir()) {
		t.Error("directory must not count as binary")
	}
}
