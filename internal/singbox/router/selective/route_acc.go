package selective

import (
	"sort"
	"sync"
)

// RouteAccumulator groups /32 CIDRs by outbound for 19-selective-routes.json.
// Only host routes are tracked — broader static CIDRs use ipset only.
type RouteAccumulator struct {
	mu         sync.Mutex
	byOutbound map[string]map[string]struct{}
}

// NewRouteAccumulator constructs an empty accumulator.
func NewRouteAccumulator() *RouteAccumulator {
	return &RouteAccumulator{byOutbound: make(map[string]map[string]struct{})}
}

// Add records a CIDR for the given outbound when it is a /32 host route.
func (a *RouteAccumulator) Add(outbound, cidr string) {
	if a == nil || outbound == "" {
		return
	}
	cidr = normalizeEntry(cidr)
	if cidr == "" || !isHostCIDR(cidr) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	set := a.byOutbound[outbound]
	if set == nil {
		set = make(map[string]struct{})
		a.byOutbound[outbound] = set
	}
	set[cidr] = struct{}{}
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
		list := make([]string, 0, len(set))
		for c := range set {
			list = append(list, c)
		}
		sort.Strings(list)
		out[ob] = list
	}
	return out
}

func isHostCIDR(cidr string) bool {
	dest := conntrackDestArg(cidr)
	return dest != ""
}
