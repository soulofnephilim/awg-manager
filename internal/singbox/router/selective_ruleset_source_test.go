package router

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoaxisr/awg-manager/internal/singbox/router/selective"
)

func TestParseDatRuleSetURL(t *testing.T) {
	kind, tags, ok := parseDatRuleSetURL("http://127.0.0.1:8080/api/singbox/router/rulesets/dat-srs?kind=geosite&tag=GOOGLE&tag=YOUTUBE&token=x")
	if !ok {
		t.Fatal("expected dat URL to parse")
	}
	if kind != "geosite" || len(tags) != 2 || tags[0] != "GOOGLE" || tags[1] != "YOUTUBE" {
		t.Fatalf("got kind=%q tags=%v", kind, tags)
	}
	if _, _, ok := parseDatRuleSetURL("https://example.com/geosite-youtube.srs"); ok {
		t.Fatal("external URL must not parse as dat")
	}
}

func TestOpenSelectiveRuleSetJSON_RemoteSRSStream(t *testing.T) {
	origDL, origDT := ruleSetDownload, ruleSetDecompileToFile
	t.Cleanup(func() { ruleSetDownload, ruleSetDecompileToFile = origDL, origDT })

	ruleSetDownload = func(_ context.Context, url, _format string) (string, error) {
		return "/tmp/fake-" + url + ".srs", nil
	}
	decompiled := t.TempDir() + "/decompiled.json"
	if err := os.WriteFile(decompiled, []byte(`{"version":3,"rules":[{"domain_suffix":["only-ruleset.example"]}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	ruleSetDecompileToFile = func(_ context.Context, _binary, _srsPath string) (string, error) {
		return decompiled, nil
	}

	s := newTestService(t, Deps{})
	path, cleanup, err := s.openSelectiveRuleSetJSON(context.Background(), selective.RuleSetRef{
		Tag:    "geosite-example",
		Type:   "remote",
		Format: "binary",
		URL:    "https://example.com/geosite-example.srs",
	})
	if err != nil {
		t.Fatalf("openSelectiveRuleSetJSON: %v", err)
	}
	defer cleanup()
	if path != decompiled {
		t.Fatalf("path = %q want decompiled file", path)
	}

	var matchers []string
	sink := selective.CollectSink{
		OnDomainQuery: func(q selective.DomainQuery) error {
			matchers = append(matchers, q.Matcher)
			return nil
		},
	}
	refs := []selective.RuleSetRef{{
		Tag:  "geosite-example",
		Type: "remote",
		URL:  "https://example.com/geosite-example.srs",
	}}
	_, errs := selective.StreamCollectFromRules(context.Background(),
		[]selective.RuleJSON{{Action: "route", Outbound: "proxy", RuleSet: []string{"geosite-example"}}},
		refs, selective.GeoPaths{}, s.OpenSelectiveRuleSetJSON, sink)
	if len(errs) > 0 {
		t.Fatalf("stream collect errors: %v", errs)
	}
	if len(matchers) != 1 || matchers[0] != "only-ruleset.example" {
		t.Fatalf("matchers = %v", matchers)
	}
}

func TestEnrichSelectiveRuleSetRefs_DatKindOnly(t *testing.T) {
	s := newTestService(t, Deps{})
	cfg := &RouterConfig{Route: Route{
		RuleSet: []RuleSet{{
			Tag:  "dat-google",
			Type: "remote",
			URL:  "http://127.0.0.1:8080/api/singbox/router/rulesets/dat-srs?kind=geosite&tag=GOOGLE",
		}},
	}}
	refs := s.enrichSelectiveRuleSetRefs(context.Background(), []selective.RuleSetRef{{
		Tag:  "dat-google",
		Type: "remote",
	}}, cfg)
	if len(refs) != 1 || refs[0].DatKind != "geosite" || len(refs[0].DatTags) != 1 || refs[0].DatTags[0] != "GOOGLE" {
		t.Fatalf("dat metadata not enriched: %+v", refs[0])
	}
	if len(refs[0].Rules) != 0 {
		t.Fatalf("rules must not be loaded into memory: %+v", refs[0].Rules)
	}
}

func TestLoadRuleSetSourceRules_LocalJSONSibling(t *testing.T) {
	dir := t.TempDir()
	srsPath := filepath.Join(dir, "myset.srs")
	jsonPath := filepath.Join(dir, "myset.json")
	if err := os.WriteFile(jsonPath, []byte(`{"version":3,"rules":[{"ip_cidr":["10.0.0.0/8"]}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(srsPath, []byte("fake"), 0644)

	s := newTestService(t, Deps{})
	rules, err := s.loadRuleSetSourceRules(context.Background(), RuleSet{
		Tag:    "myset",
		Type:   "local",
		Format: "binary",
		Path:   srsPath,
	})
	if err != nil {
		t.Fatalf("loadRuleSetSourceRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("rules = %+v", rules)
	}
}

func TestLoadRuleSetSourceRules_RemoteDecompile(t *testing.T) {
	origDL, origDC := ruleSetDownload, ruleSetDecompileExec
	t.Cleanup(func() { ruleSetDownload, ruleSetDecompileExec = origDL, origDC })

	ruleSetDownload = func(_ context.Context, url, _format string) (string, error) {
		return "/tmp/fake-" + strings.TrimPrefix(url, "https://") + ".srs", nil
	}
	ruleSetDecompileExec = func(_ context.Context, _binary, _srsPath string) ([]byte, error) {
		return []byte(`{"version":3,"rules":[{"domain_suffix":["example.com"]}]}`), nil
	}

	s := newTestService(t, Deps{})
	rules, err := s.loadRuleSetSourceRules(context.Background(), RuleSet{
		Tag:    "geosite-example",
		Type:   "remote",
		Format: "binary",
		URL:    "https://example.com/geosite-example.srs",
	})
	if err != nil {
		t.Fatalf("loadRuleSetSourceRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("rules len = %d", len(rules))
	}
	suffixes, _ := rules[0]["domain_suffix"].([]interface{})
	if len(suffixes) != 1 || suffixes[0] != "example.com" {
		t.Fatalf("domain_suffix = %v", rules[0]["domain_suffix"])
	}
}
