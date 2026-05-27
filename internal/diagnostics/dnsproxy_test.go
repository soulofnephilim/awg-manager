package diagnostics

import (
	"os"
	"path/filepath"
	"testing"
)

func loadSample(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "dns_proxy_sample.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return raw
}

func TestParseDNSProxy_ProxyCountAndNames(t *testing.T) {
	proxies, err := ParseDNSProxy(loadSample(t))
	if err != nil {
		t.Fatalf("ParseDNSProxy: %v", err)
	}
	if len(proxies) != 4 {
		t.Fatalf("want 4 proxies, got %d", len(proxies))
	}
	wantNames := []string{"System", "Policy0", "Policy1", "Policy2"}
	for i, w := range wantNames {
		if proxies[i].Name != w {
			t.Errorf("proxy[%d].Name = %q, want %q", i, proxies[i].Name, w)
		}
		if proxies[i].DisplayName != "" {
			t.Errorf("proxy[%d].DisplayName should be empty (handler fills it), got %q", i, proxies[i].DisplayName)
		}
	}
}

func TestParseDNSProxy_Ports(t *testing.T) {
	proxies, _ := ParseDNSProxy(loadSample(t))
	if proxies[0].TCPPort != 53 || proxies[0].UDPPort != 53 {
		t.Errorf("System ports = %d/%d, want 53/53", proxies[0].TCPPort, proxies[0].UDPPort)
	}
	if proxies[2].TCPPort != 41101 {
		t.Errorf("Policy1 tcpPort = %d, want 41101", proxies[2].TCPPort)
	}
}

func TestParseDNSProxy_UpstreamsAndEncryption(t *testing.T) {
	proxies, _ := ParseDNSProxy(loadSample(t))
	sys := proxies[0]
	if len(sys.Upstreams) != 3 {
		t.Fatalf("System upstreams = %d, want 3", len(sys.Upstreams))
	}
	u0 := sys.Upstreams[0]
	if u0.Address != "8.8.8.8" || u0.Encryption != "DoT" || u0.SNI != "dns.google" || u0.Scope != "all" {
		t.Errorf("u0 = %+v, want 8.8.8.8/DoT/dns.google/all", u0)
	}
	u1 := sys.Upstreams[1]
	if u1.Address != "77.88.8.8" || u1.Port != 853 || u1.Encryption != "DoT" || u1.Scope != "ru" {
		t.Errorf("u1 = %+v, want 77.88.8.8:853/DoT/ru", u1)
	}
}

func TestParseDNSProxy_StatJoinByLocalPort(t *testing.T) {
	proxies, _ := ParseDNSProxy(loadSample(t))
	sys := proxies[0]
	u1 := sys.Upstreams[1]
	if u1.RSent != 2 || u1.ARcvd != 2 || u1.MedResp != "84ms" || u1.AvgResp != "78ms" || u1.Rank != 4 {
		t.Errorf("u1 stat = %+v, want RSent2 ARcvd2 Med84ms Avg78ms Rank4", u1)
	}
	p1 := proxies[2]
	var nine DNSUpstream
	for _, u := range p1.Upstreams {
		if u.Address == "9.9.9.9" {
			nine = u
		}
	}
	if nine.RSent != 87 || nine.Rank != 4 || nine.AvgResp != "109ms" {
		t.Errorf("Policy1 9.9.9.9 stat = %+v, want RSent87 Rank4 Avg109ms", nine)
	}
}

func TestParseDNSProxy_Summary(t *testing.T) {
	proxies, _ := ParseDNSProxy(loadSample(t))
	p1 := proxies[2].Stat
	if p1.TotalRequests != 283 || p1.ProxyRequestsSent != 102 || p1.CacheHits != 181 {
		t.Errorf("Policy1 stat summary = %+v, want total283 sent102 hits181", p1)
	}
	if p1.CacheHitRatio < 0.63 || p1.CacheHitRatio > 0.65 {
		t.Errorf("Policy1 cacheHitRatio = %v, want ~0.64", p1.CacheHitRatio)
	}
	if p1.Memory != "17.25K" {
		t.Errorf("Policy1 memory = %q, want 17.25K", p1.Memory)
	}
}

func TestParseDNSProxy_StaticRecords(t *testing.T) {
	proxies, _ := ParseDNSProxy(loadSample(t))
	sys := proxies[0]
	if len(sys.Static) != 9 {
		t.Fatalf("System static records = %d, want 9", len(sys.Static))
	}
	r := sys.Static[0]
	if r.Host != "host1.example.net" || r.Type != "A" || r.Value != "203.0.113.10" || r.Flag != 1 {
		t.Errorf("static[0] = %+v", r)
	}
	var hasAAAA bool
	for _, s := range sys.Static {
		if s.Type == "AAAA" {
			hasAAAA = true
		}
	}
	if !hasAAAA {
		t.Error("expected at least one AAAA static record")
	}
}

func TestParseDNSProxy_Rebind(t *testing.T) {
	proxies, _ := ParseDNSProxy(loadSample(t))
	rb := proxies[0].Rebind
	if !rb.Enabled {
		t.Error("System rebind should be enabled")
	}
	if len(rb.Nets) != 4 {
		t.Errorf("rebind nets = %d, want 4", len(rb.Nets))
	}
	if len(rb.Excludes) != 2 || rb.Excludes[0] != "ru" || rb.Excludes[1] != "*.ru" {
		t.Errorf("rebind excludes = %v, want [ru *.ru]", rb.Excludes)
	}
}

func TestParseDNSProxy_ZeroRequestProxy(t *testing.T) {
	proxies, _ := ParseDNSProxy(loadSample(t))
	p2 := proxies[3]
	if p2.Stat.TotalRequests != 0 {
		t.Errorf("Policy2 total = %d, want 0", p2.Stat.TotalRequests)
	}
	if len(p2.Upstreams) != 3 {
		t.Errorf("Policy2 upstreams = %d, want 3", len(p2.Upstreams))
	}
}

func TestParseDNSProxy_EmptyInput(t *testing.T) {
	proxies, err := ParseDNSProxy([]byte(`{"proxy-status":[]}`))
	if err != nil {
		t.Fatalf("empty status should not error: %v", err)
	}
	if len(proxies) != 0 {
		t.Errorf("want 0 proxies, got %d", len(proxies))
	}
}

// TestParseDNSProxy_DoHEncryption verifies that applyEncryption correctly marks
// an upstream as DoH when its address appears in proxy-https.server-https, and
// that DoH wins over DoT when the address appears in both lists.
func TestParseDNSProxy_DoHEncryption(t *testing.T) {
	const raw = `{"proxy-status":[{
		"proxy-name": "System",
		"proxy-config": "dns_server = 127.0.0.1:40500 . # 1.1.1.1\ndns_server = 127.0.0.1:40501 . # 8.8.8.8\ndns_server = 127.0.0.1:40502 . # 9.9.9.9",
		"proxy-stat": "",
		"proxy-tls":  {"server-tls":  [{"address":"9.9.9.9","port":853,"sni":"dns.quad9.net","domain":""}]},
		"proxy-https":{"server-https":[{"address":"1.1.1.1"},{"address":"9.9.9.9"}]}
	}]}`

	proxies, err := ParseDNSProxy([]byte(raw))
	if err != nil {
		t.Fatalf("ParseDNSProxy: %v", err)
	}
	if len(proxies) != 1 || len(proxies[0].Upstreams) != 3 {
		t.Fatalf("unexpected shape: %+v", proxies)
	}
	byAddr := map[string]DNSUpstream{}
	for _, u := range proxies[0].Upstreams {
		byAddr[u.Address] = u
	}

	// 1.1.1.1 — DoH only
	if u := byAddr["1.1.1.1"]; u.Encryption != "DoH" {
		t.Errorf("1.1.1.1: want DoH, got %q", u.Encryption)
	}
	// 8.8.8.8 — neither list → plain
	if u := byAddr["8.8.8.8"]; u.Encryption != "plain" {
		t.Errorf("8.8.8.8: want plain, got %q", u.Encryption)
	}
	// 9.9.9.9 — in both lists: DoH must win
	if u := byAddr["9.9.9.9"]; u.Encryption != "DoH" {
		t.Errorf("9.9.9.9: want DoH (DoH>DoT), got %q", u.Encryption)
	}
}
