package selective

import (
	"context"
	"slices"
	"testing"
)

// ── extractHost ───────────────────────────────────────────────────────────────

func TestExtractHost_BareIP(t *testing.T) {
	if got := extractHost("8.8.8.8"); got != "8.8.8.8" {
		t.Errorf("got %q", got)
	}
}

func TestExtractHost_IPWithPort(t *testing.T) {
	if got := extractHost("8.8.8.8:53"); got != "8.8.8.8" {
		t.Errorf("got %q", got)
	}
}

func TestExtractHost_TLSScheme(t *testing.T) {
	if got := extractHost("tls://1.1.1.1"); got != "1.1.1.1" {
		t.Errorf("got %q", got)
	}
}

func TestExtractHost_HTTPSWithPath(t *testing.T) {
	if got := extractHost("https://dns.example.com/dns-query"); got != "dns.example.com" {
		t.Errorf("got %q", got)
	}
}

func TestExtractHost_Empty(t *testing.T) {
	if got := extractHost(""); got != "" {
		t.Errorf("got %q", got)
	}
}

// ── parseNDMSUpstreamAddresses ────────────────────────────────────────────────

func TestParseNDMSUpstreamAddresses_ArrayShape(t *testing.T) {
	raw := []byte(`[{"upstreams":[{"address":"1.1.1.1"},{"address":"8.8.8.8"}]}]`)
	got := parseNDMSUpstreamAddresses(raw)
	if !slices.Contains(got, "1.1.1.1") || !slices.Contains(got, "8.8.8.8") {
		t.Errorf("expected 1.1.1.1 and 8.8.8.8, got %v", got)
	}
}

func TestParseNDMSUpstreamAddresses_SingleShape(t *testing.T) {
	raw := []byte(`{"upstreams":[{"address":"9.9.9.9"}]}`)
	got := parseNDMSUpstreamAddresses(raw)
	if !slices.Contains(got, "9.9.9.9") {
		t.Errorf("expected 9.9.9.9, got %v", got)
	}
}

func TestParseNDMSUpstreamAddresses_DedupesAddresses(t *testing.T) {
	raw := []byte(`[{"upstreams":[{"address":"1.1.1.1"},{"address":"1.1.1.1"}]}]`)
	got := parseNDMSUpstreamAddresses(raw)
	count := 0
	for _, a := range got {
		if a == "1.1.1.1" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 occurrence of 1.1.1.1, got %d in %v", count, got)
	}
}

func TestParseNDMSUpstreamAddresses_SkipsIPv6(t *testing.T) {
	raw := []byte(`[{"upstreams":[{"address":"::1"},{"address":"1.1.1.1"}]}]`)
	got := parseNDMSUpstreamAddresses(raw)
	for _, a := range got {
		if a == "::1" {
			t.Errorf("IPv6 should be skipped, got %v", got)
		}
	}
	if !slices.Contains(got, "1.1.1.1") {
		t.Errorf("expected 1.1.1.1, got %v", got)
	}
}

func TestParseNDMSUpstreamAddresses_BOM(t *testing.T) {
	// Some NDMS firmwares include a UTF-8 BOM.
	raw := append([]byte{0xEF, 0xBB, 0xBF}, `[{"upstreams":[{"address":"1.2.3.4"}]}]`...)
	got := parseNDMSUpstreamAddresses(raw)
	if !slices.Contains(got, "1.2.3.4") {
		t.Errorf("expected 1.2.3.4, got %v", got)
	}
}

func TestParseNDMSUpstreamAddresses_EmptyResponse(t *testing.T) {
	got := parseNDMSUpstreamAddresses(nil)
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestParseNDMSUpstreamAddresses_Garbage(t *testing.T) {
	got := parseNDMSUpstreamAddresses([]byte(`not json`))
	if len(got) != 0 {
		t.Errorf("expected empty on garbage input, got %v", got)
	}
}

// ── BuildDNSServers — priority logic ─────────────────────────────────────────

type fakeDNSSource struct {
	raw []byte
	err error
}

func (f *fakeDNSSource) List(_ context.Context) ([]byte, error) {
	return f.raw, f.err
}

func TestBuildDNSServers_UnionsSingboxNDMSAndPublic(t *testing.T) {
	servers := []SingboxDNSServer{
		{Tag: "google", Type: "udp", Server: "8.8.8.8"},
	}
	ndms := &fakeDNSSource{raw: []byte(`[{"upstreams":[{"address":"1.1.1.1"}]}]`)}
	got := BuildDNSServers(context.Background(), servers, ndms)
	for _, want := range []string{"8.8.8.8", "1.1.1.1", "9.9.9.9", "77.88.8.8"} {
		if !slices.Contains(got, want) {
			t.Errorf("expected %s in union, got %v", want, got)
		}
	}
}

func TestBuildDNSServers_IncludesNDMSAndPublic(t *testing.T) {
	servers := []SingboxDNSServer{
		{Tag: "local", Type: "local", Server: ""},
	}
	ndms := &fakeDNSSource{raw: []byte(`[{"upstreams":[{"address":"9.9.9.9"}]}]`)}
	got := BuildDNSServers(context.Background(), servers, ndms)
	if !slices.Contains(got, "9.9.9.9") {
		t.Errorf("expected NDMS 9.9.9.9, got %v", got)
	}
	if !slices.Contains(got, "1.1.1.1") {
		t.Errorf("expected public fallback 1.1.1.1, got %v", got)
	}
}

func TestExpandQueryHosts_Suffix(t *testing.T) {
	hosts := expandQueryHosts("2ip.ru", KindDomainSuffix, true)
	if !slices.Contains(hosts, "2ip.ru") || !slices.Contains(hosts, "www.2ip.ru") {
		t.Fatalf("expected apex and www, got %v", hosts)
	}
	if !slices.Contains(hosts, "cdn.2ip.ru") {
		t.Errorf("expected CDN probe host, got %v", hosts)
	}
}

func TestExpandQueryHosts_SuffixMinimalProbes(t *testing.T) {
	hosts := expandQueryHosts("2ip.ru", KindDomainSuffix, false)
	if !slices.Contains(hosts, "2ip.ru") || !slices.Contains(hosts, "www.2ip.ru") {
		t.Fatalf("expected apex and www, got %v", hosts)
	}
	if slices.Contains(hosts, "cdn.2ip.ru") {
		t.Errorf("minimal mode must not probe cdn., got %v", hosts)
	}
	if len(hosts) != 1+len(minimalSuffixProbes) {
		t.Errorf("expected %d hosts, got %v", 1+len(minimalSuffixProbes), hosts)
	}
}

func TestExpandQueryHosts_Exact(t *testing.T) {
	hosts := expandQueryHosts("www.example.com", KindDomain, true)
	if len(hosts) != 1 || hosts[0] != "www.example.com" {
		t.Fatalf("expected single exact host, got %v", hosts)
	}
}

func TestFullProbeFlags(t *testing.T) {
	// A hand-written suffix rule, an exact rule, then a geosite-scale tail:
	// the first budget's worth of suffix matchers keep full probes, the
	// tail is minimal, and exact-domain queries neither consume the budget
	// nor get demoted.
	queries := []DomainQuery{
		{Matcher: "hand.example", Kind: KindDomainSuffix},
		{Matcher: "exact.example", Kind: KindDomain},
	}
	for i := 0; i < fullProbeSuffixBudget+10; i++ {
		queries = append(queries, DomainQuery{Matcher: "geo.example", Kind: KindDomainSuffix})
	}
	flags := fullProbeFlags(queries)
	if !flags[0] {
		t.Error("first (hand-written) suffix matcher must keep full probes")
	}
	if !flags[1] {
		t.Error("exact-domain query must always be full (single host anyway)")
	}
	// Budget continues for geosite entries 2..fullProbeSuffixBudget, then stops.
	if !flags[2] {
		t.Error("suffix matchers within budget must keep full probes")
	}
	if flags[len(flags)-1] {
		t.Error("suffix matchers past the budget must fall back to minimal probes")
	}
	full := 0
	for i, f := range flags {
		if f && queries[i].Kind != KindDomain {
			full++
		}
	}
	if full != fullProbeSuffixBudget {
		t.Errorf("exactly %d suffix matchers must keep full probes, got %d", fullProbeSuffixBudget, full)
	}
}

func TestBuildDNSServers_EmptyWhenOnlyGarbageNDMS(t *testing.T) {
	got := BuildDNSServers(context.Background(), nil, &fakeDNSSource{raw: []byte(`[]`)})
	// Public fallbacks are always present for rebuild sampling.
	if len(got) < len(publicDNSFallbacks) {
		t.Errorf("expected at least public fallbacks, got %v", got)
	}
}

func TestBuildDNSServers_SkipsLocalAndFakeip(t *testing.T) {
	servers := []SingboxDNSServer{
		{Tag: "local", Type: "local", Server: ""},
		{Tag: "fakeip", Type: "fakeip", Server: ""},
		{Tag: "real", Type: "udp", Server: "1.0.0.1"},
	}
	got := BuildDNSServers(context.Background(), servers, nil)
	if !slices.Contains(got, "1.0.0.1") {
		t.Errorf("expected 1.0.0.1, got %v", got)
	}
	if !slices.Contains(got, "8.8.8.8") {
		t.Errorf("expected public fallback 8.8.8.8, got %v", got)
	}
}

func TestBuildDNSServers_StripsTLSScheme(t *testing.T) {
	servers := []SingboxDNSServer{
		{Tag: "dot", Type: "tls", Server: "tls://1.1.1.1"},
	}
	got := BuildDNSServers(context.Background(), servers, nil)
	if !slices.Contains(got, "1.1.1.1") {
		t.Errorf("expected 1.1.1.1 (stripped), got %v", got)
	}
}

func TestBuildDNSServers_NilNDMSSource(t *testing.T) {
	got := BuildDNSServers(context.Background(), nil, nil)
	if len(got) < len(publicDNSFallbacks) {
		t.Errorf("expected public fallbacks, got %v", got)
	}
}
