package router

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/hoaxisr/awg-manager/internal/singbox/orchestrator"
)

const selectiveIPRuleManaged = "selective-ip"

// selectiveRoutesSlot is the JSON shape for 19-selective-routes.json.
// Only route.rules — never inbounds/outbounds; null slices in a merged
// slot corrupt sing-box's outbounds array (unknown outbound type at index N).
type selectiveRoutesSlot struct {
	Route struct {
		Rules []Rule `json:"rules"`
	} `json:"route"`
}

func marshalSelectiveRoutesSlot(rules []Rule) ([]byte, error) {
	slot := selectiveRoutesSlot{}
	slot.Route.Rules = rules
	return json.MarshalIndent(slot, "", "  ")
}

// buildSelectiveIPRules builds ip_cidr route rules from outbound-grouped /32 lists.
func buildSelectiveIPRules(byOutbound map[string][]string) []Rule {
	if len(byOutbound) == 0 {
		return nil
	}
	out := make([]Rule, 0, len(byOutbound))
	for ob, cidrs := range byOutbound {
		out = append(out, Rule{
			Action:   "route",
			Outbound: ob,
			IPCIDR:   cidrs,
			// AwgmManaged intentionally omitted: 19-selective-routes.json is
			// merged into sing-box config and must not contain unknown fields.
		})
	}
	return out
}

// stripSelectiveManagedRules removes auto-generated selective-ip rules that were
// mistakenly persisted into 20-router.json by an earlier build.
func stripSelectiveManagedRules(rules []Rule) ([]Rule, bool) {
	if len(rules) == 0 {
		return rules, false
	}
	filtered := rules[:0]
	changed := false
	for _, r := range rules {
		if r.AwgmManaged == selectiveIPRuleManaged {
			changed = true
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered, changed
}

// stripLegacySelectiveRulesFromRouter removes managed selective-ip rules from
// the user-visible router slot (20-router.json). Returns whether anything changed.
func (s *ServiceImpl) stripLegacySelectiveRulesFromRouter(ctx context.Context) (bool, error) {
	cfg, err := s.loadRouterConfig()
	if err != nil {
		return false, err
	}
	cfg = s.ruleSetMaterializer().restoreConfig(cfg)
	filtered, changed := stripSelectiveManagedRules(cfg.Route.Rules)
	if !changed {
		return false, nil
	}
	cfg.Route.Rules = filtered
	if err := s.persistConfigDirect(ctx, cfg); err != nil {
		return false, err
	}
	s.emitCfgEvent("rules", cfg)
	return true, nil
}

// syncSelectiveRoutesSlot writes ip_cidr overlay rules into 19-selective-routes.json
// (merged by sing-box, invisible in the rules UI). Uses SaveSilent so no staging
// draft / «Применить» banner appears after ipset rebuild.
func (s *ServiceImpl) syncSelectiveRoutesSlot(ctx context.Context, byOutbound map[string][]string) error {
	if s.deps.Orch == nil {
		return nil
	}
	rules := buildSelectiveIPRules(byOutbound)
	enable := len(rules) > 0
	data, err := marshalSelectiveRoutesSlot(rules)
	if err != nil {
		return err
	}
	// Always rewrite the slot file so a legacy build's `"outbounds": null`
	// cannot keep breaking sing-box check after rules are cleared.
	if err := s.deps.Orch.SaveSilent(orchestrator.SlotSelectiveRoutes, data); err != nil {
		return err
	}
	return s.deps.Orch.SetEnabledSilent(orchestrator.SlotSelectiveRoutes, enable)
}

// healLegacySelectiveRoutesSlotIfNeeded rewrites 19-selective-routes.json when an
// older build left unsupported keys (outbounds/inbounds null, awgm_managed).
func (s *ServiceImpl) healLegacySelectiveRoutesSlotIfNeeded(ctx context.Context) error {
	if s.deps.Orch == nil {
		return nil
	}
	path := filepath.Join(s.deps.Orch.ConfigDir(), "19-selective-routes.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if !selectiveRoutesSlotNeedsHeal(data) {
		return nil
	}
	var slot selectiveRoutesSlot
	if err := json.Unmarshal(data, &slot); err != nil {
		var legacy RouterConfig
		if err2 := json.Unmarshal(data, &legacy); err2 != nil {
			return s.syncSelectiveRoutesSlot(ctx, nil)
		}
		slot.Route.Rules = legacy.Route.Rules
	}
	for i := range slot.Route.Rules {
		slot.Route.Rules[i].AwgmManaged = ""
	}
	fixed, err := marshalSelectiveRoutesSlot(slot.Route.Rules)
	if err != nil {
		return err
	}
	enable := len(slot.Route.Rules) > 0
	if err := s.deps.Orch.SaveSilent(orchestrator.SlotSelectiveRoutes, fixed); err != nil {
		return err
	}
	return s.deps.Orch.SetEnabledSilent(orchestrator.SlotSelectiveRoutes, enable)
}

func selectiveRoutesSlotNeedsHeal(data []byte) bool {
	s := string(data)
	return bytesContainsLegacyNullSlotKeys(data) ||
		strings.Contains(s, `"awgm_managed"`)
}

func bytesContainsLegacyNullSlotKeys(data []byte) bool {
	s := string(data)
	return strings.Contains(s, `"outbounds"`) || strings.Contains(s, `"inbounds"`)
}

// disableSelectiveRoutesSlot turns off the overlay slot when selective bypass
// is disabled.
func (s *ServiceImpl) disableSelectiveRoutesSlot() error {
	if s.deps.Orch == nil {
		return nil
	}
	return s.deps.Orch.SetEnabledSilent(orchestrator.SlotSelectiveRoutes, false)
}

func normalizeSelectiveIPCIDR(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if _, ipnet, err := net.ParseCIDR(raw); err == nil {
		if ipnet.IP.To4() == nil {
			return ""
		}
		return ipnet.String()
	}
	if ip := net.ParseIP(raw); ip != nil && ip.To4() != nil {
		return ip.To4().String() + "/32"
	}
	return ""
}
