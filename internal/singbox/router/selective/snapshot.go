package selective

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

// DomainResolveResult is one rule matcher resolved to IPv4 /32 CIDRs for ipset.
type DomainResolveResult struct {
	// Matcher is the domain or suffix as written in the routing rule (cleaned).
	Matcher string `json:"matcher"`
	// Kind is "domain" (exact) or "suffix" (domain_suffix).
	Kind string `json:"kind"`
	// QueryHosts lists every hostname that was queried (suffix rules expand to
	// apex + common CDN subdomains).
	QueryHosts []string `json:"queryHosts"`
	// IPs are no longer persisted or returned via API (ipset holds them).
	IPs []string `json:"ips,omitempty"`
	// CDN is true when any resolved IP falls in a known anycast CDN prefix.
	// Such matchers are included in the 20-minute background refresh loop.
	CDN bool `json:"cdn,omitempty"`
	// Outbound is the proxy outbound tag from the source route rule.
	Outbound string `json:"outbound,omitempty"`
	// Error is set when no IP could be resolved for any query host.
	Error string `json:"error,omitempty"`
}

// RebuildSnapshot is the persisted outcome of the last successful ipset rebuild.
// Written beside config.d (same parent dir as selective-last-rebuild) — NOT as
// *.json inside config.d, or sing-box merge will try to decode it and FATAL.
type RebuildSnapshot struct {
	RebuiltAt          string                `json:"rebuiltAt"`
	StaticCIDRs        []string              `json:"staticCidrs"`
	DomainResults      []DomainResolveResult `json:"domainResults"`
	EntryCount         int                   `json:"entryCount"`
	StaticCIDRCount    int                   `json:"staticCidrCount,omitempty"`
	DomainMatcherCount int                   `json:"domainMatcherCount,omitempty"`
	LastCDNRefresh     string                `json:"lastCDNRefresh,omitempty"`
}

// snapshotFileName has no .json suffix: sing-box -C config.d merges every
// *.json in that directory; our metadata must not look like a config fragment.
const snapshotFileName = "selective-snapshot"

// legacySnapshotJSON is the old path (pre-2.14.2 fix) that broke sing-box startup.
const legacySnapshotJSON = "selective-snapshot.json"

func snapshotPath(configDir string) string {
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, snapshotFileName)
}

func legacySnapshotPath(configDir string) string {
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, legacySnapshotJSON)
}

// RemoveLegacySnapshotJSON deletes selective-snapshot.json from config.d if a
// previous build left it there (sing-box FATAL: unknown field "rebuiltAt").
func RemoveLegacySnapshotJSON(configDir string) {
	p := legacySnapshotPath(configDir)
	if p == "" {
		return
	}
	_ = os.Remove(p)
}

// NeedsPopulation reports whether the selective ipset should be built on daemon
// start: set missing, no successful rebuild marker on disk, or an empty set the
// last rebuild expected to have entries. An empty set whose persisted snapshot
// says the last rebuild produced 0 entries was built empty deliberately (user
// has no IP/domain matchers) — re-running a full rebuild on every daemon start
// for that case is pure boot-time contention.
func NeedsPopulation(ctx context.Context, configDir string) bool {
	if !SetExists(ctx) {
		return true
	}
	return needsPopulationForExistingSet(EntryCount(ctx), readLastRebuild(configDir), readSnapshotSummary(configDir))
}

// needsPopulationForExistingSet is the NeedsPopulation decision once the set
// is known to exist. Split out so the empty-set logic is testable without a
// live ipset.
func needsPopulationForExistingSet(entryCount int, lastRebuild time.Time, summary *SnapshotSummary) bool {
	if lastRebuild.IsZero() {
		return true
	}
	if entryCount > 0 {
		return false
	}
	// Empty set: intentional ONLY when the last rebuild had nothing
	// configured at all (no static CIDRs, no domain matchers). A summary
	// with matchers but 0 entries means the rebuild «succeeded» during a
	// DNS/WAN outage (resolve failures are not errors) — treating that as
	// deliberate would leave the set empty forever and leak proxied traffic
	// via WAN. No snapshot, or one that expected entries, means the set lost
	// its contents — rebuild in all these cases.
	return summary == nil ||
		summary.EntryCount != 0 ||
		summary.StaticCIDRCount > 0 ||
		summary.DomainMatcherCount > 0
}

// NormalizeRebuildSnapshot ensures slice fields are non-nil so JSON encodes
// [] instead of null (the frontend snapshot UI reads .length on these fields).
func NormalizeRebuildSnapshot(snap *RebuildSnapshot) *RebuildSnapshot {
	if snap == nil {
		return nil
	}
	cp := *snap
	if cp.StaticCIDRs == nil {
		cp.StaticCIDRs = []string{}
	}
	if cp.DomainResults == nil {
		cp.DomainResults = []DomainResolveResult{}
	}
	for i := range cp.DomainResults {
		if cp.DomainResults[i].QueryHosts == nil {
			cp.DomainResults[i].QueryHosts = []string{}
		}
		if cp.DomainResults[i].IPs == nil {
			cp.DomainResults[i].IPs = []string{}
		}
	}
	return &cp
}

func writeSnapshot(configDir string, snap RebuildSnapshot) {
	summary := SnapshotSummary{
		RebuiltAt:          snap.RebuiltAt,
		EntryCount:         snap.EntryCount,
		StaticCIDRCount:    len(snap.StaticCIDRs),
		DomainMatcherCount: len(snap.DomainResults),
		LastCDNRefresh:     snap.LastCDNRefresh,
	}
	writeSnapshotMeta(configDir, summary)
	RemoveLegacySnapshotJSON(configDir)
}
