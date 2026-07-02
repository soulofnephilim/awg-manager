package router

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoaxisr/awg-manager/internal/singbox/orchestrator"
)

func TestBuildSelectiveIPRules_GroupsByOutbound(t *testing.T) {
	rules := buildSelectiveIPRules(map[string][]string{
		"awg-tunnel": {"188.40.167.82/32"},
	})
	if len(rules) != 1 {
		t.Fatalf("got %d rules", len(rules))
	}
	if rules[0].Outbound != "awg-tunnel" || rules[0].IPCIDR[0] != "188.40.167.82/32" {
		t.Fatalf("unexpected rule: %+v", rules[0])
	}
}

func TestStripSelectiveManagedRules(t *testing.T) {
	rules := []Rule{
		{AwgmManaged: selectiveIPRuleManaged, Action: "route", Outbound: "x", IPCIDR: []string{"1.2.3.4/32"}},
		{Action: "route", Outbound: "direct", DomainSuffix: []string{"example.com"}},
	}
	filtered, changed := stripSelectiveManagedRules(rules)
	if !changed || len(filtered) != 1 || filtered[0].DomainSuffix[0] != "example.com" {
		t.Fatalf("got %+v changed=%v", filtered, changed)
	}
}

func TestMarshalSelectiveRoutesSlot_RouteOnly(t *testing.T) {
	data, err := marshalSelectiveRoutesSlot(nil)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"outbounds", "inbounds", "dns"} {
		if _, ok := m[forbidden]; ok {
			t.Fatalf("slot JSON must not contain %q: %s", forbidden, string(data))
		}
	}
}

func TestHealLegacySelectiveRoutesSlotIfNeeded(t *testing.T) {
	dir := t.TempDir()
	orch := orchestrator.New(dir, nil)
	_ = orch.Register(orchestrator.SlotMeta{Slot: orchestrator.SlotSelectiveRoutes, Filename: "19-selective-routes.json"})
	_ = orch.SetEnabled(orchestrator.SlotSelectiveRoutes, true)

	broken := []byte(`{
  "inbounds": null,
  "outbounds": null,
  "route": {"rules": [{"action": "route", "outbound": "tun", "ip_cidr": ["1.2.3.4/32"]}]}
}`)
	if err := os.WriteFile(filepath.Join(dir, "19-selective-routes.json"), broken, 0644); err != nil {
		t.Fatal(err)
	}

	svc := &ServiceImpl{deps: Deps{Orch: orch}}
	if err := svc.healLegacySelectiveRoutesSlotIfNeeded(context.Background()); err != nil {
		t.Fatal(err)
	}
	fixed, err := os.ReadFile(filepath.Join(dir, "19-selective-routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytesContainsLegacyNullSlotKeys(fixed) {
		t.Fatalf("file still broken: %s", fixed)
	}
}

func TestHealLegacySelectiveRoutesSlot_StripsAwgmManaged(t *testing.T) {
	dir := t.TempDir()
	orch := orchestrator.New(dir, nil)
	_ = orch.Register(orchestrator.SlotMeta{Slot: orchestrator.SlotSelectiveRoutes, Filename: "19-selective-routes.json"})
	_ = orch.SetEnabled(orchestrator.SlotSelectiveRoutes, true)

	broken := []byte(`{
  "route": {"rules": [{"action": "route", "outbound": "tun", "ip_cidr": ["1.2.3.4/32"], "awgm_managed": "selective-ip"}]}
}`)
	if err := os.WriteFile(filepath.Join(dir, "19-selective-routes.json"), broken, 0644); err != nil {
		t.Fatal(err)
	}

	svc := &ServiceImpl{deps: Deps{Orch: orch}}
	if err := svc.healLegacySelectiveRoutesSlotIfNeeded(context.Background()); err != nil {
		t.Fatal(err)
	}
	fixed, err := os.ReadFile(filepath.Join(dir, "19-selective-routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(fixed), "awgm_managed") {
		t.Fatalf("awgm_managed still present: %s", fixed)
	}
}

func TestStripLegacySelectiveRulesFromRouter(t *testing.T) {
	dir := t.TempDir()
	orch := orchestrator.New(dir, nil)
	_ = orch.Register(orchestrator.SlotMeta{Slot: orchestrator.SlotRouter, Filename: "20-router.json"})
	_ = orch.SetEnabled(orchestrator.SlotRouter, true)

	cfg := NewEmptyConfig()
	cfg.Route.Rules = []Rule{
		{AwgmManaged: selectiveIPRuleManaged, Action: "route", Outbound: "tun", IPCIDR: []string{"188.40.167.82/32"}},
		{Action: "route", Outbound: "tun", DomainSuffix: []string{"2ip.ru"}},
	}
	svc := &ServiceImpl{deps: Deps{Orch: orch}}
	if err := svc.persistConfigDirect(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	changed, err := svc.stripLegacySelectiveRulesFromRouter(context.Background())
	if err != nil || !changed {
		t.Fatalf("strip: changed=%v err=%v", changed, err)
	}
	rules, err := svc.ListRules(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].DomainSuffix[0] != "2ip.ru" {
		t.Fatalf("unexpected rules after strip: %+v", rules)
	}
}
