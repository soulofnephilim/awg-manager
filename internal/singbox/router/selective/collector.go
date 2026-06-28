package selective

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// DomainKind identifies how a matcher should be resolved for ipset population.
type DomainKind string

const (
	KindDomain       DomainKind = "domain"
	KindDomainSuffix DomainKind = "suffix"
)

// DomainQuery is one domain matcher from a route rule or rule-set.
type DomainQuery struct {
	Matcher  string     `json:"matcher"`
	Kind     DomainKind `json:"kind"`
	Outbound string     `json:"outbound,omitempty"`
}

// CollectResult holds the raw output of a collection pass over the router
// rules and rule sets.
type CollectResult struct {
	// CIDRs are already-valid IPv4 CIDR strings extracted from ip_cidr
	// matchers in rules and rule sets.
	CIDRs []string
	// DomainQueries lists domain / domain_suffix matchers that need DNS
	// resolution before they can be added to ipset.
	DomainQueries []DomainQuery
	// Errors lists non-fatal errors encountered while reading rule set files
	// (e.g. a remote rule set whose .srs sidecar JSON is missing). The
	// collection continues with the remaining sets.
	Errors []error
}

// RuleJSON is a minimal representation of a sing-box route rule used for
// collecting matchers. Fields not relevant to ipset building are ignored.
// Exported so the router adapter can convert router.Rule → RuleJSON without
// importing router types into the selective package (import cycle).
type RuleJSON struct {
	Action       string     `json:"action,omitempty"`
	Outbound     string     `json:"outbound,omitempty"`
	IPCIDR       []string   `json:"ip_cidr"`
	DomainSuffix []string   `json:"domain_suffix"`
	Domain       []string   `json:"domain"`
	RuleSet      []string   `json:"rule_set"`
	Rules        []RuleJSON `json:"rules"` // logical rule nesting
}

// ruleSetSourceJSON is the on-disk shape of a compiled inline rule set
// source file (<tag>.json alongside the <tag>.srs binary).
type ruleSetSourceJSON struct {
	Version int                      `json:"version"`
	Rules   []map[string]interface{} `json:"rules"`
}

// RuleSetRef describes a rule set entry, carrying enough information for
// the collector to locate its source JSON.
type RuleSetRef struct {
	Tag  string // rule set tag as referenced in rules
	Type string // "inline", "local", "remote"
	Path string // on-disk path (for local/materialized); empty for remote
	URL    string // remote rule-set URL (for download/decompile)
	Format string // "binary" or "source"
	// InlineDir is the directory where inline rule set JSONs are compiled to.
	InlineDir string
	// DatDir is the directory where dat-derived rule set JSONs are written.
	DatDir string
	// DatKind and DatTags identify geosite/geoip dat rule-sets (streamed from .dat).
	DatKind string
	DatTags []string
	// Rules carries the rule set's matchers in memory when they are already
	// known (e.g. an inline set restored by the router's materializer). When
	// non-empty the collector reads these directly and skips the on-disk JSON
	// lookup — more robust than re-reading a sidecar that may be missing,
	// stale, or named differently. Shape mirrors sing-box rule-set source
	// rules (map per rule with ip_cidr / domain_suffix / domain keys).
	Rules []map[string]interface{}
}

// CollectFromRules walks route rules and collects IPv4 CIDR / domain matchers
// that belong to proxy (non-direct) route rules only. Rule sets are expanded
// only when referenced by such a rule — never from the full rule_set catalog.
func CollectFromRules(rules []RuleJSON, ruleSetRefs []RuleSetRef) CollectResult {
	var result CollectResult
	seen := &deduplicator{}
	proxySetTags := make(map[string]string)

	for i := range rules {
		collectFromRule(&rules[i], "", seen, &result, proxySetTags)
	}

	if len(proxySetTags) == 0 {
		return result
	}

	refsByTag := make(map[string]RuleSetRef, len(ruleSetRefs))
	for _, ref := range ruleSetRefs {
		refsByTag[ref.Tag] = ref
	}
	for tag, outbound := range proxySetTags {
		ref, ok := refsByTag[tag]
		if !ok {
			continue
		}
		if err := collectFromRuleSetRef(ref, outbound, seen, &result); err != nil {
			result.Errors = append(result.Errors, err)
		}
	}

	return result
}

// isProxyRoute reports whether traffic matched by this rule should be sent to
// sing-box in selective mode (non-direct proxy outbound).
func isProxyRoute(r *RuleJSON, outbound string) bool {
	if r.Action != "" && r.Action != "route" {
		return false
	}
	ob := outbound
	if r.Outbound != "" {
		ob = r.Outbound
	}
	return ob != "" && ob != "direct"
}

func effectiveOutbound(r *RuleJSON, parent string) string {
	if r.Outbound != "" {
		return r.Outbound
	}
	return parent
}

func collectFromRule(r *RuleJSON, parentOutbound string, seen *deduplicator, result *CollectResult, proxySetTags map[string]string) {
	outbound := effectiveOutbound(r, parentOutbound)
	if isProxyRoute(r, outbound) {
		for _, cidr := range r.IPCIDR {
			if c := normalizeCIDR(cidr); c != "" && seen.addCIDR(c) {
				result.CIDRs = append(result.CIDRs, c)
			}
		}
		for _, d := range r.DomainSuffix {
			if d = cleanDomain(d); d != "" && seen.addDomainQuery(d, KindDomainSuffix, outbound) {
				result.DomainQueries = append(result.DomainQueries, DomainQuery{Matcher: d, Kind: KindDomainSuffix, Outbound: outbound})
			}
		}
		for _, d := range r.Domain {
			if d = cleanDomain(d); d != "" && seen.addDomainQuery(d, KindDomain, outbound) {
				result.DomainQueries = append(result.DomainQueries, DomainQuery{Matcher: d, Kind: KindDomain, Outbound: outbound})
			}
		}
		for _, tag := range r.RuleSet {
			if tag == "" {
				continue
			}
			if _, ok := proxySetTags[tag]; !ok {
				proxySetTags[tag] = outbound
			}
		}
	}
	for i := range r.Rules {
		collectFromRule(&r.Rules[i], outbound, seen, result, proxySetTags)
	}
}

// collectFromRuleSetRef loads the source JSON for a rule set and extracts
// ip_cidr and domain entries from its rules. outbound is taken from the proxy
// route rule that referenced this set.
func collectFromRuleSetRef(ref RuleSetRef, outbound string, seen *deduplicator, result *CollectResult) error {
	if len(ref.Rules) > 0 {
		for _, ruleMap := range ref.Rules {
			extractFromRuleMap(ruleMap, seen, result, outbound)
		}
		return nil
	}

	jsonPath := resolveRuleSetJSONPath(ref)
	if jsonPath == "" {
		return nil
	}
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var src ruleSetSourceJSON
	if err := json.Unmarshal(raw, &src); err != nil {
		return err
	}
	for _, ruleMap := range src.Rules {
		extractFromRuleMap(ruleMap, seen, result, outbound)
	}
	return nil
}

// resolveRuleSetJSONPath returns the path to the source JSON file for a
// rule set, or "" if it cannot be determined.
func resolveRuleSetJSONPath(ref RuleSetRef) string {
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
		return filepath.Join(ref.InlineDir, safeFilename(ref.Tag)+".json")
	case "local":
		return ""
	case "remote":
		if ref.DatDir == "" {
			return ""
		}
		return filepath.Join(ref.DatDir, safeFilename(ref.Tag)+".json")
	}
	return ""
}

// normalizeCIDR validates and canonicalises an IPv4 CIDR string.
// Returns "" for IPv6 or invalid input.
func normalizeCIDR(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if _, ipnet, err := net.ParseCIDR(s); err == nil {
		if ipnet.IP.To4() == nil {
			return ""
		}
		return ipnet.String()
	}
	if ip := net.ParseIP(s); ip != nil && ip.To4() != nil {
		return ip.To4().String() + "/32"
	}
	return ""
}

// cleanDomain strips leading dots/wildcards and lowercases the domain.
func cleanDomain(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, ".")
	s = strings.TrimPrefix(s, "*.")
	return s
}

// toStringSlice coerces an interface{} value that may be []interface{}
// or []string into a []string. Returns nil for other types.
func toStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []string:
		return val
	case []interface{}:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// safeFilename replaces characters not suitable for filenames with "-",
// mirroring the safeRuleSetFilename logic in the router package.
func safeFilename(tag string) string {
	var b strings.Builder
	for _, r := range tag {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "ruleset"
	}
	return result
}

// deduplicator tracks seen CIDRs and domain queries to avoid duplicates.
type deduplicator struct {
	cidrs         map[string]struct{}
	domainQueries map[string]string // key → outbound
}

func (d *deduplicator) addCIDR(s string) bool {
	if d.cidrs == nil {
		d.cidrs = make(map[string]struct{})
	}
	if _, ok := d.cidrs[s]; ok {
		return false
	}
	d.cidrs[s] = struct{}{}
	return true
}

func (d *deduplicator) addDomainQuery(matcher string, kind DomainKind, outbound string) bool {
	if d.domainQueries == nil {
		d.domainQueries = make(map[string]string)
	}
	key := string(kind) + "\x00" + matcher
	if ob, ok := d.domainQueries[key]; ok {
		if ob == "" && outbound != "" {
			d.domainQueries[key] = outbound
		}
		return false
	}
	d.domainQueries[key] = outbound
	return true
}

// extractFromRuleMap pulls ip_cidr and domain entries from the raw
// map[string]interface{} form used in rule set source JSON files.
func extractFromRuleMap(m map[string]interface{}, seen *deduplicator, result *CollectResult, outbound string) {
	for _, key := range []string{"ip_cidr"} {
		v, _ := m[key]
		for _, s := range toStringSlice(v) {
			if c := normalizeCIDR(s); c != "" && seen.addCIDR(c) {
				result.CIDRs = append(result.CIDRs, c)
			}
		}
	}
	for _, key := range []string{"domain_suffix", "domain"} {
		kind := KindDomain
		if key == "domain_suffix" {
			kind = KindDomainSuffix
		}
		v, _ := m[key]
		for _, s := range toStringSlice(v) {
			if d := cleanDomain(s); d != "" && seen.addDomainQuery(d, kind, outbound) {
				result.DomainQueries = append(result.DomainQueries, DomainQuery{Matcher: d, Kind: kind, Outbound: outbound})
			}
		}
	}
}
