package selective

import (
	"net"
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

// RuleSetRef describes a rule set entry, carrying enough information for
// the collector to locate its source JSON.
type RuleSetRef struct {
	Tag    string // rule set tag as referenced in rules
	Type   string // "inline", "local", "remote"
	Path   string // on-disk path (for local/materialized); empty for remote
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
