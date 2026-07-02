package router

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hoaxisr/awg-manager/internal/singbox/router/selective"
)

// selectiveBuilderAdapter implements SelectiveBuilder by pulling the current
// router config (rules + rule-sets) and DNS servers from the live service,
// then delegating to the underlying selective.Builder.
type selectiveBuilderAdapter struct {
	svc *ServiceImpl
	b   *selective.Builder
}

// NewSelectiveBuilderAdapter constructs a SelectiveBuilder backed by svc and b.
func NewSelectiveBuilderAdapter(svc *ServiceImpl, b *selective.Builder) SelectiveBuilder {
	return &selectiveBuilderAdapter{svc: svc, b: b}
}

// Rebuild loads the current router config from disk, derives rule-set JSON
// paths, and calls the underlying Builder.Rebuild.
func (a *selectiveBuilderAdapter) Rebuild(ctx context.Context) error {
	cfg, err := a.svc.loadRouterConfig()
	if err != nil {
		return err
	}
	cfg = a.svc.ruleSetMaterializer().restoreConfig(cfg)

	rules := rulesAsSelectiveJSON(cfg.Route.Rules)

	configDir := ""
	if a.svc.deps.Singbox != nil {
		configDir = a.svc.deps.Singbox.ConfigDir()
	}
	refs := a.svc.enrichSelectiveRuleSetRefs(ctx, ruleSetRefsFromConfig(cfg, configDir), cfg)
	singboxDNS := singboxDNSServersFromConfig(cfg)

	a.svc.appLog.Info("selective-rebuild", "",
		fmt.Sprintf("starting rebuild: %d rules, %d rule-sets, %d dns-servers",
			len(rules), len(refs), len(singboxDNS)),
	)

	if err := a.b.Rebuild(ctx, rules, refs, singboxDNS, nil); err != nil {
		return err
	}
	a.syncRoutesAfterRebuild(ctx)
	return nil
}

func (a *selectiveBuilderAdapter) syncRoutesAfterRebuild(ctx context.Context) {
	if _, err := a.svc.stripLegacySelectiveRulesFromRouter(ctx); err != nil {
		a.svc.appLog.Warn("selective-rebuild", "routes", err.Error())
	}
	routes := a.b.LastIPRulesByOutbound()
	entryCount := selective.EntryCount(ctx)
	if entryCount > 0 && len(routes) == 0 {
		a.svc.appLog.Warn("selective-rebuild", "routes",
			fmt.Sprintf("ipset has %d entries but no /32 overlay routes — traffic may reach sing-box yet exit via route.final", entryCount))
	}
	if err := a.svc.syncSelectiveRoutesSlot(ctx, routes); err != nil {
		a.svc.appLog.Warn("selective-rebuild", "routes", err.Error())
		return
	}
	if err := a.svc.orchestratorApplyNow(); err != nil {
		a.svc.appLog.Warn("selective-rebuild", "routes", err.Error())
	}
}

// RefreshCDN incrementally refreshes ipset entries for CDN-flagged domain matchers.
func (a *selectiveBuilderAdapter) RefreshCDN(ctx context.Context) error {
	configDir := ""
	if a.svc.deps.Singbox != nil {
		configDir = a.svc.deps.Singbox.ConfigDir()
	}
	queries, err := selective.CDNQueriesFromConfigDir(configDir)
	if err != nil || len(queries) == 0 {
		return err
	}

	cfg, err := a.svc.loadRouterConfig()
	if err != nil {
		return err
	}
	cfg = a.svc.ruleSetMaterializer().restoreConfig(cfg)
	singboxDNS := singboxDNSServersFromConfig(cfg)

	newRoutes, err := a.b.RefreshCDNMatchers(ctx, queries, singboxDNS)
	if err != nil {
		return err
	}
	// Re-sync the routes slot (and reload sing-box) ONLY when the refresh
	// actually produced new /32 overlay routes. ipset additions take effect
	// in the kernel immediately; an unconditional sync here meant a SIGHUP —
	// or a full stop+start with a tun inbound — every 20 minutes, dropping
	// all proxied connections even when nothing changed.
	if newRoutes > 0 {
		a.syncRoutesAfterRebuild(ctx)
	}
	return nil
}

func rulesAsSelectiveJSON(rules []Rule) []selective.RuleJSON {
	userRules, _ := stripSelectiveManagedRules(rules)
	out := make([]selective.RuleJSON, len(userRules))
	for i, r := range userRules {
		out[i] = selective.RuleJSON{
			Action:       r.Action,
			Outbound:     r.Outbound,
			IPCIDR:       r.IPCIDR,
			Domain:       r.Domain,
			DomainSuffix: r.DomainSuffix,
			RuleSet:      r.RuleSet,
			Rules:        rulesAsSelectiveJSON(r.Rules),
		}
	}
	return out
}

func ruleSetRefsFromConfig(cfg *RouterConfig, configDir string) []selective.RuleSetRef {
	if cfg == nil {
		return nil
	}
	inlineDir := filepath.Join(configDir, "rule-sets", "inline")
	datDir := filepath.Join(configDir, "rule-sets", "dat")

	refs := make([]selective.RuleSetRef, 0, len(cfg.Route.RuleSet))
	for _, rs := range cfg.Route.RuleSet {
		ref := selective.RuleSetRef{
			Tag:       rs.Tag,
			Type:      rs.Type,
			Path:      rs.Path,
			URL:       rs.URL,
			Format:    rs.Format,
			InlineDir: inlineDir,
			DatDir:    datDir,
		}
		if kind, tags, ok := parseDatRuleSetURL(rs.URL); ok {
			ref.DatKind = kind
			ref.DatTags = tags
		}
		if len(rs.Rules) > 0 {
			ref.Rules = append(ref.Rules, rs.Rules...)
		}
		refs = append(refs, ref)
	}
	return refs
}

func singboxDNSServersFromConfig(cfg *RouterConfig) []selective.SingboxDNSServer {
	if cfg == nil {
		return nil
	}
	out := make([]selective.SingboxDNSServer, 0, len(cfg.DNS.Servers))
	for _, srv := range cfg.DNS.Servers {
		out = append(out, selective.SingboxDNSServer{
			Tag:    srv.Tag,
			Type:   srv.Type,
			Server: srv.Server,
		})
	}
	return out
}

// OpenSelectiveRuleSetJSON implements selective.RuleSetJSONOpener.
func (s *ServiceImpl) OpenSelectiveRuleSetJSON(ctx context.Context, ref selective.RuleSetRef) (string, func(), error) {
	return s.openSelectiveRuleSetJSON(ctx, ref)
}

func selectiveJSONPath(ref selective.RuleSetRef) string {
	if ref.Path != "" {
		if strings.HasSuffix(strings.ToLower(ref.Path), ".json") {
			return ref.Path
		}
		return strings.TrimSuffix(ref.Path, ".srs") + ".json"
	}
	switch ref.Type {
	case "inline":
		if ref.InlineDir == "" {
			return ""
		}
		return filepath.Join(ref.InlineDir, safeRuleSetFilename(ref.Tag)+".json")
	case "remote":
		if ref.DatDir == "" {
			return ""
		}
		return filepath.Join(ref.DatDir, safeRuleSetFilename(ref.Tag)+".json")
	}
	return ""
}
