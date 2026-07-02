package selective

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// collectedResult mirrors the shape the old batch collector returned so the
// semantic assertions below stay readable. Populated via a collecting sink
// over StreamCollectFromRules — the ONE production collection path.
type collectedResult struct {
	CIDRs         []string
	DomainQueries []DomainQuery
	Errors        []error
}

func collectAll(t *testing.T, rules []RuleJSON, refs []RuleSetRef) collectedResult {
	t.Helper()
	var res collectedResult
	res.Errors = StreamCollectFromRules(context.Background(), rules, refs, GeoPaths{}, nil, CollectSink{
		OnStaticCIDR: func(cidr string) error {
			res.CIDRs = append(res.CIDRs, cidr)
			return nil
		},
		OnDomainQuery: func(q DomainQuery) error {
			res.DomainQueries = append(res.DomainQueries, q)
			return nil
		},
	})
	return res
}

func matchersOf(res collectedResult) []string {
	out := make([]string, 0, len(res.DomainQueries))
	for _, q := range res.DomainQueries {
		out = append(out, q.Matcher)
	}
	return out
}

func hasMatcher(res collectedResult, matcher string) bool {
	return slices.Contains(matchersOf(res), matcher)
}

func proxyRule(r RuleJSON) RuleJSON {
	r.Action = "route"
	if r.Outbound == "" {
		r.Outbound = "proxy"
	}
	return r
}

// testRuleSetSourceJSON is the on-disk shape of a rule-set source file
// (sing-box source format) used to write test fixtures.
type testRuleSetSourceJSON struct {
	Version int                      `json:"version"`
	Rules   []map[string]interface{} `json:"rules"`
}

func mustWriteJSON(t *testing.T, dir, name string, v any) string {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// ── normalizeCIDR ─────────────────────────────────────────────────────────────

func TestNormalizeCIDR_ValidCIDR(t *testing.T) {
	if got := normalizeCIDR("10.0.0.0/8"); got != "10.0.0.0/8" {
		t.Errorf("got %q", got)
	}
}

func TestNormalizeCIDR_HostBitsMasked(t *testing.T) {
	// net.ParseCIDR canonicalises: 10.0.0.1/8 → 10.0.0.0/8
	if got := normalizeCIDR("10.0.0.1/8"); got != "10.0.0.0/8" {
		t.Errorf("got %q", got)
	}
}

func TestNormalizeCIDR_BareIP(t *testing.T) {
	if got := normalizeCIDR("1.2.3.4"); got != "1.2.3.4/32" {
		t.Errorf("got %q", got)
	}
}

func TestNormalizeCIDR_IPv6Rejected(t *testing.T) {
	if got := normalizeCIDR("::1/128"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestNormalizeCIDR_Garbage(t *testing.T) {
	if got := normalizeCIDR("not-an-ip"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// ── cleanDomain ───────────────────────────────────────────────────────────────

func TestCleanDomain_StripLeadingDot(t *testing.T) {
	if got := cleanDomain(".example.com"); got != "example.com" {
		t.Errorf("got %q", got)
	}
}

func TestCleanDomain_StripWildcard(t *testing.T) {
	if got := cleanDomain("*.example.com"); got != "example.com" {
		t.Errorf("got %q", got)
	}
}

func TestCleanDomain_Lowercase(t *testing.T) {
	if got := cleanDomain("EXAMPLE.COM"); got != "example.com" {
		t.Errorf("got %q", got)
	}
}

// ── StreamCollectFromRules — direct rules ─────────────────────────────────────

func TestCollect_IPCIDRExtracted(t *testing.T) {
	rules := []RuleJSON{
		proxyRule(RuleJSON{IPCIDR: []string{"1.2.3.0/24", "5.6.7.8"}}),
	}
	res := collectAll(t, rules, nil)
	if !slices.Contains(res.CIDRs, "1.2.3.0/24") {
		t.Errorf("missing CIDR 1.2.3.0/24, got %v", res.CIDRs)
	}
	if !slices.Contains(res.CIDRs, "5.6.7.8/32") {
		t.Errorf("missing CIDR 5.6.7.8/32, got %v", res.CIDRs)
	}
}

func TestCollect_DomainsExtracted(t *testing.T) {
	rules := []RuleJSON{
		proxyRule(RuleJSON{DomainSuffix: []string{"example.com", ".foo.com"}}),
		proxyRule(RuleJSON{Domain: []string{"*.bar.com", "baz.com"}}),
	}
	res := collectAll(t, rules, nil)
	for _, want := range []string{"example.com", "foo.com", "bar.com", "baz.com"} {
		if !hasMatcher(res, want) {
			t.Errorf("missing domain %q, got %v", want, res.DomainQueries)
		}
	}
}

func TestCollect_IPv6Skipped(t *testing.T) {
	rules := []RuleJSON{proxyRule(RuleJSON{IPCIDR: []string{"::1/128", "10.0.0.1/32"}})}
	res := collectAll(t, rules, nil)
	if slices.Contains(res.CIDRs, "::1/128") {
		t.Errorf("IPv6 should be skipped, got %v", res.CIDRs)
	}
	if !slices.Contains(res.CIDRs, "10.0.0.1/32") {
		t.Errorf("missing 10.0.0.1/32, got %v", res.CIDRs)
	}
}

func TestCollect_DeduplicatesCIDRs(t *testing.T) {
	rules := []RuleJSON{
		proxyRule(RuleJSON{IPCIDR: []string{"1.2.3.0/24", "1.2.3.0/24"}}),
	}
	res := collectAll(t, rules, nil)
	count := 0
	for _, c := range res.CIDRs {
		if c == "1.2.3.0/24" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 occurrence of 1.2.3.0/24, got %d", count)
	}
}

func TestCollect_DeduplicatesDomains(t *testing.T) {
	rules := []RuleJSON{
		proxyRule(RuleJSON{DomainSuffix: []string{"example.com"}}),
		proxyRule(RuleJSON{DomainSuffix: []string{"example.com"}}),
	}
	res := collectAll(t, rules, nil)
	count := 0
	for _, q := range res.DomainQueries {
		if q.Matcher == "example.com" && q.Kind == KindDomainSuffix {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 suffix query for example.com, got %d", count)
	}
}

func TestCollect_NestedLogicalRules(t *testing.T) {
	rules := []RuleJSON{
		proxyRule(RuleJSON{
			Rules: []RuleJSON{
				{IPCIDR: []string{"10.20.30.0/24"}},
				{DomainSuffix: []string{"nested.com"}},
			},
		}),
	}
	res := collectAll(t, rules, nil)
	if !slices.Contains(res.CIDRs, "10.20.30.0/24") {
		t.Errorf("nested CIDR not collected, got %v", res.CIDRs)
	}
	if !hasMatcher(res, "nested.com") {
		t.Errorf("nested domain not collected, got %v", res.DomainQueries)
	}
}

func TestCollect_DirectRulesSkipped(t *testing.T) {
	rules := []RuleJSON{
		{Action: "route", Outbound: "direct", DomainSuffix: []string{"github.com"}, IPCIDR: []string{"1.1.1.1"}},
	}
	res := collectAll(t, rules, nil)
	if len(res.CIDRs) != 0 || len(res.DomainQueries) != 0 {
		t.Fatalf("direct rules must not populate ipset, got %+v", res)
	}
}

func TestCollect_UnreferencedRuleSetsSkipped(t *testing.T) {
	refs := []RuleSetRef{
		{
			Tag:  "orphan",
			Type: "inline",
			Rules: []map[string]interface{}{
				{"domain_suffix": []interface{}{"orphan.example"}},
			},
		},
	}
	res := collectAll(t, nil, refs)
	if len(res.CIDRs) != 0 || len(res.DomainQueries) != 0 {
		t.Fatalf("unreferenced rule sets must be ignored, got %+v", res)
	}
}

func TestCollect_EmptyRules(t *testing.T) {
	res := collectAll(t, nil, nil)
	if len(res.CIDRs) != 0 || len(res.DomainQueries) != 0 || len(res.Errors) != 0 {
		t.Errorf("expected empty result, got %+v", res)
	}
}

// ── StreamCollectFromRules — inline rule set ──────────────────────────────────

func TestCollect_RuleSetRefInMemoryRules(t *testing.T) {
	refs := []RuleSetRef{
		{
			Tag:  "custom-1",
			Type: "inline",
			Rules: []map[string]interface{}{
				{"domain_suffix": []interface{}{"2ip.ru"}},
				{"ip_cidr": []interface{}{"203.0.113.0/24"}},
			},
		},
	}
	res := collectAll(t, []RuleJSON{
		proxyRule(RuleJSON{RuleSet: []string{"custom-1"}}),
	}, refs)
	if !hasMatcher(res, "2ip.ru") {
		t.Errorf("missing in-memory domain, got %v", res.DomainQueries)
	}
	if !slices.Contains(res.CIDRs, "203.0.113.0/24") {
		t.Errorf("missing in-memory CIDR, got %v", res.CIDRs)
	}
}

// When a ref carries in-memory rules, the on-disk JSON is ignored entirely.
func TestCollect_RuleSetRefInMemoryRulesIgnoresDisk(t *testing.T) {
	dir := t.TempDir()
	mustWriteJSON(t, dir, "custom-1.json", testRuleSetSourceJSON{
		Version: 1,
		Rules:   []map[string]interface{}{{"domain_suffix": []interface{}{"from-disk.example"}}},
	})
	refs := []RuleSetRef{
		{
			Tag:       "custom-1",
			Type:      "inline",
			InlineDir: dir,
			Rules:     []map[string]interface{}{{"domain_suffix": []interface{}{"from-memory.example"}}},
		},
	}
	res := collectAll(t, []RuleJSON{
		proxyRule(RuleJSON{RuleSet: []string{"custom-1"}}),
	}, refs)
	if !hasMatcher(res, "from-memory.example") {
		t.Errorf("expected in-memory domain, got %v", res.DomainQueries)
	}
	if hasMatcher(res, "from-disk.example") {
		t.Errorf("on-disk domain should be ignored when Rules present, got %v", res.DomainQueries)
	}
}

func TestCollect_InlineRuleSet(t *testing.T) {
	dir := t.TempDir()
	src := testRuleSetSourceJSON{
		Version: 5,
		Rules: []map[string]interface{}{
			{"ip_cidr": []interface{}{"192.168.100.0/24"}},
			{"domain_suffix": []interface{}{"inline-domain.com"}},
		},
	}
	mustWriteJSON(t, dir, "myset.json", src)

	refs := []RuleSetRef{
		{Tag: "myset", Type: "inline", InlineDir: dir},
	}
	res := collectAll(t, []RuleJSON{
		proxyRule(RuleJSON{RuleSet: []string{"myset"}}),
	}, refs)
	if !slices.Contains(res.CIDRs, "192.168.100.0/24") {
		t.Errorf("missing CIDR from inline ruleset, got %v", res.CIDRs)
	}
	if !hasMatcher(res, "inline-domain.com") {
		t.Errorf("missing domain from inline ruleset, got %v", res.DomainQueries)
	}
}

func TestCollect_LocalRuleSet(t *testing.T) {
	dir := t.TempDir()
	src := testRuleSetSourceJSON{
		Version: 5,
		Rules: []map[string]interface{}{
			{"ip_cidr": []interface{}{"172.16.0.0/12"}},
		},
	}
	srsPath := filepath.Join(dir, "local-set.srs")
	mustWriteJSON(t, dir, "local-set.json", src)

	refs := []RuleSetRef{
		{Tag: "local-set", Type: "local", Path: srsPath},
	}
	res := collectAll(t, []RuleJSON{
		proxyRule(RuleJSON{RuleSet: []string{"local-set"}}),
	}, refs)
	if !slices.Contains(res.CIDRs, "172.16.0.0/12") {
		t.Errorf("missing CIDR from local ruleset, got %v", res.CIDRs)
	}
}

func TestCollect_MissingRuleSetFileSkipped(t *testing.T) {
	refs := []RuleSetRef{
		{Tag: "nonexistent", Type: "inline", InlineDir: "/tmp/nonexistent-dir-xyz"},
	}
	res := collectAll(t, []RuleJSON{
		proxyRule(RuleJSON{RuleSet: []string{"nonexistent"}}),
	}, refs)
	if len(res.Errors) != 0 {
		t.Errorf("expected no errors for missing file, got %v", res.Errors)
	}
}

func TestCollect_RemoteRuleSetNoDir_Skipped(t *testing.T) {
	refs := []RuleSetRef{
		{Tag: "google", Type: "remote"},
	}
	res := collectAll(t, []RuleJSON{
		proxyRule(RuleJSON{RuleSet: []string{"google"}}),
	}, refs)
	if len(res.Errors) != 0 || len(res.CIDRs) != 0 {
		t.Errorf("expected empty result for remote with no DatDir, got %+v", res)
	}
}

func TestCollect_RuleSetAndProxyRuleCombined(t *testing.T) {
	dir := t.TempDir()
	src := testRuleSetSourceJSON{
		Version: 5,
		Rules:   []map[string]interface{}{{"ip_cidr": []interface{}{"203.0.113.0/24"}}},
	}
	mustWriteJSON(t, dir, "my-set.json", src)

	rules := []RuleJSON{
		proxyRule(RuleJSON{IPCIDR: []string{"8.8.8.8"}, RuleSet: []string{"my-set"}}),
	}
	refs := []RuleSetRef{{Tag: "my-set", Type: "inline", InlineDir: dir}}

	res := collectAll(t, rules, refs)
	if !slices.Contains(res.CIDRs, "8.8.8.8/32") {
		t.Errorf("missing proxy rule CIDR, got %v", res.CIDRs)
	}
	if !slices.Contains(res.CIDRs, "203.0.113.0/24") {
		t.Errorf("missing ruleset CIDR, got %v", res.CIDRs)
	}
}
