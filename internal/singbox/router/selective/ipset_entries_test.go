package selective

import "testing"

func TestBuildIpsetEntries_DedupesHosts(t *testing.T) {
	static := []string{"192.168.1.0/24"}
	domains := []DomainResolveResult{
		{
			Matcher: "deepl.com",
			Kind:    "suffix",
			CDN:     true,
			IPs:     []string{"104.18.36.122/32", "104.18.39.209/32"},
		},
		{
			Matcher: "2ip.ru",
			Kind:    "suffix",
			CDN:     false,
			IPs:     []string{"188.40.167.82/32"},
		},
	}
	got := BuildIpsetEntries(static, domains)
	if len(got) != 4 {
		t.Fatalf("expected 4 entries, got %v", got)
	}
	seen := map[string]bool{}
	for _, e := range got {
		seen[e] = true
	}
	if !seen["192.168.1.0/24"] || !seen["104.18.36.122/32"] || !seen["104.18.39.209/32"] || !seen["188.40.167.82/32"] {
		t.Fatalf("unexpected set: %v", got)
	}
}

func TestConntrackDestArg(t *testing.T) {
	if got := conntrackDestArg("8.47.69.0/24"); got != "" {
		t.Fatalf("expected skip for /24, got %q", got)
	}
	if got := conntrackDestArg("1.2.3.4/32"); got != "1.2.3.4/32" {
		t.Fatalf("got %q", got)
	}
	if got := conntrackDestArg("10.0.0.0/8"); got != "" {
		t.Fatalf("expected skip for /8, got %q", got)
	}
}
