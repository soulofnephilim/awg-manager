package selective

import (
	"net/netip"
	"sort"
	"strings"
	"sync"
)

// maxSelectiveRoutes caps the number of /32 host routes accumulated for the
// 19-selective-routes.json overlay, aligned with setMaxElem/4 (262144/4).
// Unlike ipset entries — which live in the kernel — the overlay stays
// resident in the daemon after rebuild AND is marshalled into sing-box
// config, so a runaway resolve must truncate it (loudly), not exhaust RAM.
const maxSelectiveRoutes = 65_536

// RouteAccumulator groups /32 host addresses by outbound for
// 19-selective-routes.json. Only host routes are tracked — broader static
// CIDRs use ipset only.
//
// Addresses are stored as netip.Addr rather than "a.b.c.d/32" strings: the
// accumulated set stays alive after rebuild (Builder.routes feeds
// LastIPRulesByOutbound and the CDN refresh merge), and string+map storage
// cost several times the resident bytes of the packed value type. Strings
// are rendered on demand by RulesByOutbound.
type RouteAccumulator struct {
	mu         sync.Mutex
	byOutbound map[string]map[netip.Addr]struct{}
	total      int
	dropped    int
}

// NewRouteAccumulator constructs an empty accumulator.
func NewRouteAccumulator() *RouteAccumulator {
	return &RouteAccumulator{byOutbound: make(map[string]map[netip.Addr]struct{})}
}

// Add records a CIDR for the given outbound when it is a /32 host route.
// Once maxSelectiveRoutes distinct addresses are held, further new addresses
// are dropped and counted (Dropped) so the builder can surface truncation.
func (a *RouteAccumulator) Add(outbound, cidr string) {
	if a == nil || outbound == "" {
		return
	}
	addr, ok := hostRouteAddr(cidr)
	if !ok {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	set := a.byOutbound[outbound]
	if set != nil {
		if _, exists := set[addr]; exists {
			return
		}
	}
	if a.total >= maxSelectiveRoutes {
		a.dropped++
		return
	}
	// Create the per-outbound set only AFTER the budget check passes: creating
	// it first left an empty set behind when every address for a new outbound
	// was budget-dropped, and an empty set renders an empty ip_cidr list —
	// buildSelectiveIPRules then marshals a condition-less
	// {"action":"route","outbound":X} rule that sing-box either rejects
	// (reload failure) or treats as match-all for X.
	if set == nil {
		set = make(map[netip.Addr]struct{})
		a.byOutbound[outbound] = set
	}
	set[addr] = struct{}{}
	a.total++
}

// Dropped reports how many distinct host routes were rejected by the
// maxSelectiveRoutes budget.
func (a *RouteAccumulator) Dropped() int {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.dropped
}

// RulesByOutbound returns outbound → deduplicated /32 CIDR lists. Each list
// is sorted so the marshalled routes slot is byte-stable across rebuilds —
// the caller diffs slot bytes to decide whether sing-box needs a reload, and
// random map order would report «changed» on every identical rebuild.
func (a *RouteAccumulator) RulesByOutbound() map[string][]string {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string][]string, len(a.byOutbound))
	for ob, set := range a.byOutbound {
		if len(set) == 0 {
			// Defensive: an empty list would become a condition-less rule in
			// buildSelectiveIPRules — see Add.
			continue
		}
		out[ob] = renderHostRoutes(set)
	}
	return out
}

// AddrsByOutbound returns a copy of the compact netip representation for
// resident storage (Builder.routes).
func (a *RouteAccumulator) AddrsByOutbound() map[string]map[netip.Addr]struct{} {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.byOutbound) == 0 {
		return nil
	}
	out := make(map[string]map[netip.Addr]struct{}, len(a.byOutbound))
	for ob, set := range a.byOutbound {
		if len(set) == 0 {
			continue // defensive — see RulesByOutbound
		}
		cp := make(map[netip.Addr]struct{}, len(set))
		for addr := range set {
			cp[addr] = struct{}{}
		}
		out[ob] = cp
	}
	return out
}

// renderHostRoutes renders a host-address set as sorted "/32" CIDR strings.
// Lexicographic string sort keeps the output byte-identical to the historical
// string-set implementation the routes-slot byte-diff was tuned against.
func renderHostRoutes(set map[netip.Addr]struct{}) []string {
	list := make([]string, 0, len(set))
	for addr := range set {
		list = append(list, addr.String()+"/32")
	}
	sort.Strings(list)
	return list
}

// hostRouteAddr returns the IPv4 address for a /32 host CIDR (or bare IPv4),
// or ok=false for broader CIDRs, IPv6 and garbage.
func hostRouteAddr(raw string) (netip.Addr, bool) {
	entry := normalizeEntry(raw)
	if entry == "" || !strings.HasSuffix(entry, "/32") {
		return netip.Addr{}, false
	}
	addr, err := netip.ParseAddr(strings.TrimSuffix(entry, "/32"))
	if err != nil || !addr.Is4() {
		return netip.Addr{}, false
	}
	return addr, true
}
