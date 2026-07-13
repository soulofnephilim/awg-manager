package router

import (
	"strings"
	"testing"
)

func dialIssueMessages(cfg *RouterConfig) []string {
	var out []string
	for _, i := range computeDNSDialIssues(cfg) {
		out = append(out, i.Message)
	}
	return out
}

func requireIssueContaining(t *testing.T, msgs []string, substr string) {
	t.Helper()
	for _, m := range msgs {
		if strings.Contains(m, substr) {
			return
		}
	}
	t.Fatalf("no issue containing %q, got %v", substr, msgs)
}

// Доменный адрес без domain_resolver в конфиге с несколькими серверами —
// предупреждение; IP-адрес и единственный сервер — нет.
func TestDNSDialIssues_DomainWithoutResolver(t *testing.T) {
	cfg := NewEmptyConfig()
	cfg.DNS.Servers = []DNSServer{
		{Tag: "dot", Type: "tls", Server: "common.dot.dns.yandex.net"},
		{Tag: "direct", Type: "udp", Server: "77.88.8.8"},
	}
	msgs := dialIssueMessages(cfg)
	requireIssueContaining(t, msgs, `"dot" задан доменом`)
	for _, m := range msgs {
		if strings.Contains(m, `"direct"`) {
			t.Fatalf("IP-addressed server must not be flagged: %v", msgs)
		}
	}

	// Единственный сервер — по документации резолвер опционален.
	cfg.DNS.Servers = cfg.DNS.Servers[:1]
	if msgs := dialIssueMessages(cfg); len(msgs) != 0 {
		t.Fatalf("single-server config must not warn, got %v", msgs)
	}
}

// domain_resolver или route.default_domain_resolver снимают предупреждение.
func TestDNSDialIssues_ResolverSatisfies(t *testing.T) {
	cfg := NewEmptyConfig()
	cfg.DNS.Servers = []DNSServer{
		{Tag: "dot", Type: "tls", Server: "dns.example.com", DomainResolver: &DomainResolver{Server: "direct"}},
		{Tag: "direct", Type: "udp", Server: "77.88.8.8"},
	}
	if msgs := dialIssueMessages(cfg); len(msgs) != 0 {
		t.Fatalf("domain_resolver set — no warnings expected, got %v", msgs)
	}

	cfg.DNS.Servers[0].DomainResolver = nil
	cfg.Route.DefaultDomainResolver = &DomainResolver{Server: "direct"}
	if msgs := dialIssueMessages(cfg); len(msgs) != 0 {
		t.Fatalf("default_domain_resolver set — no warnings expected, got %v", msgs)
	}
}

// Ссылки на несуществующие серверы — и в domain_resolver, и в
// route.default_domain_resolver.
func TestDNSDialIssues_UnknownResolverRefs(t *testing.T) {
	cfg := NewEmptyConfig()
	cfg.DNS.Servers = []DNSServer{
		{Tag: "dot", Type: "tls", Server: "dns.example.com", DomainResolver: &DomainResolver{Server: "ghost"}},
		{Tag: "direct", Type: "udp", Server: "77.88.8.8"},
	}
	cfg.Route.DefaultDomainResolver = &DomainResolver{Server: "phantom"}
	msgs := dialIssueMessages(cfg)
	requireIssueContaining(t, msgs, `domain_resolver ссылается на несуществующий DNS-сервер "ghost"`)
	requireIssueContaining(t, msgs, `default_domain_resolver ссылается на несуществующий DNS-сервер "phantom"`)
}

// domain_strategy на сервере — удалено в sing-box 1.14.
func TestDNSDialIssues_DeprecatedDomainStrategy(t *testing.T) {
	cfg := NewEmptyConfig()
	cfg.DNS.Servers = []DNSServer{
		{Tag: "direct", Type: "udp", Server: "77.88.8.8", Strategy: "ipv4_only"},
	}
	requireIssueContaining(t, dialIssueMessages(cfg), "domain_strategy — поле удалено")
}

// detour + domain_resolver: dial-поля при detour игнорируются.
func TestDNSDialIssues_DetourIgnoresDialFields(t *testing.T) {
	cfg := NewEmptyConfig()
	cfg.DNS.Servers = []DNSServer{
		{Tag: "remote", Type: "udp", Server: "1.1.1.1", Detour: "vpn", DomainResolver: &DomainResolver{Server: "direct"}},
		{Tag: "direct", Type: "udp", Server: "77.88.8.8"},
	}
	requireIssueContaining(t, dialIssueMessages(cfg), "при заданном detour остальные dial-поля игнорируются")
}

// Цикл резолверов: dot → doh → dot.
func TestDNSDialIssues_ResolverCycle(t *testing.T) {
	cfg := NewEmptyConfig()
	cfg.DNS.Servers = []DNSServer{
		{Tag: "dot", Type: "tls", Server: "dot.example.com", DomainResolver: &DomainResolver{Server: "doh"}},
		{Tag: "doh", Type: "https", Server: "doh.example.com", DomainResolver: &DomainResolver{Server: "dot"}},
	}
	msgs := dialIssueMessages(cfg)
	requireIssueContaining(t, msgs, "цикл domain_resolver")

	// Самозацикливание.
	cfg.DNS.Servers = []DNSServer{
		{Tag: "dot", Type: "tls", Server: "dot.example.com", DomainResolver: &DomainResolver{Server: "dot"}},
		{Tag: "direct", Type: "udp", Server: "77.88.8.8"},
	}
	requireIssueContaining(t, dialIssueMessages(cfg), "цикл domain_resolver")

	// Цепочка, оканчивающаяся IP-сервером, — не цикл.
	cfg.DNS.Servers = []DNSServer{
		{Tag: "dot", Type: "tls", Server: "dot.example.com", DomainResolver: &DomainResolver{Server: "direct"}},
		{Tag: "direct", Type: "udp", Server: "77.88.8.8"},
	}
	for _, m := range dialIssueMessages(cfg) {
		if strings.Contains(m, "цикл") {
			t.Fatalf("IP-terminated chain is not a cycle: %v", dialIssueMessages(cfg))
		}
	}
}

// Цикл через default_domain_resolver: доменный сервер без своего резолвера
// падает в default, указывающий на него самого.
func TestDNSDialIssues_CycleViaDefaultResolver(t *testing.T) {
	cfg := NewEmptyConfig()
	cfg.DNS.Servers = []DNSServer{
		{Tag: "dot", Type: "tls", Server: "dot.example.com"},
		{Tag: "direct", Type: "udp", Server: "77.88.8.8"},
	}
	cfg.Route.DefaultDomainResolver = &DomainResolver{Server: "dot"}
	requireIssueContaining(t, dialIssueMessages(cfg), "цикл domain_resolver")
}

// local/fakeip не имеют upstream-адреса — dial-проверки не применяются;
// IPv6-литерал в скобках — не домен.
func TestDNSDialIssues_NonUpstreamAndIPv6(t *testing.T) {
	cfg := NewEmptyConfig()
	cfg.DNS.Servers = []DNSServer{
		{Tag: "loc", Type: "local"},
		{Tag: "fake", Type: "fakeip", Inet4Range: "198.18.0.0/15"},
		{Tag: "v6", Type: "udp", Server: "[2a02:6b8::feed:0ff]"},
	}
	if msgs := dialIssueMessages(cfg); len(msgs) != 0 {
		t.Fatalf("no warnings expected, got %v", msgs)
	}
}

// computeIssues прокидывает dial-предупреждения в общий список.
func TestComputeIssues_IncludesDNSDialIssues(t *testing.T) {
	svc := &ServiceImpl{deps: Deps{}}
	cfg := NewEmptyConfig()
	cfg.DNS.Servers = []DNSServer{
		{Tag: "dot", Type: "tls", Server: "dns.example.com"},
		{Tag: "direct", Type: "udp", Server: "77.88.8.8"},
	}
	found := false
	for _, i := range svc.computeIssues(cfg) {
		if i.Kind == "dns-domain-resolver" {
			found = true
		}
	}
	if !found {
		t.Fatal("computeIssues must include dns-domain-resolver warnings")
	}
}
