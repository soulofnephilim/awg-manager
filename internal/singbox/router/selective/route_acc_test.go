package selective

import (
	"fmt"
	"slices"
	"testing"
)

// TestRouteAccumulator_SortedLexicographicOutput pins the rendering contract:
// RulesByOutbound must return the same lexicographically sorted "/32" strings
// the historical string-set implementation produced — the routes-slot
// byte-diff (skip-SIGHUP optimisation) depends on it. Note "10.x" sorting
// BEFORE "2.x": string order, not numeric.
func TestRouteAccumulator_SortedLexicographicOutput(t *testing.T) {
	acc := NewRouteAccumulator()
	for _, ip := range []string{"10.0.0.2/32", "2.0.0.1", "1.2.3.4/32", "10.0.0.2"} {
		acc.Add("vpn", ip)
	}
	got := acc.RulesByOutbound()["vpn"]
	want := []string{"1.2.3.4/32", "10.0.0.2/32", "2.0.0.1/32"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRouteAccumulator_SkipsNonHostEntries(t *testing.T) {
	acc := NewRouteAccumulator()
	acc.Add("vpn", "10.0.0.0/8") // broader than /32 — ipset only
	acc.Add("vpn", "::1")        // IPv6
	acc.Add("", "1.1.1.1/32")    // no outbound
	acc.Add("vpn", "not-an-ip")  // garbage
	if got := acc.RulesByOutbound(); len(got["vpn"]) != 0 {
		t.Fatalf("expected no routes, got %v", got)
	}
	if acc.Dropped() != 0 {
		t.Fatalf("skips must not count as budget drops, got %d", acc.Dropped())
	}
}

func TestRouteAccumulator_Budget(t *testing.T) {
	acc := NewRouteAccumulator()
	const extra = 100
	for i := 0; i < maxSelectiveRoutes+extra; i++ {
		acc.Add("vpn", fmt.Sprintf("%d.%d.%d.%d/32", 10+(i>>24), (i>>16)&0xff, (i>>8)&0xff, i&0xff))
	}
	if got := len(acc.RulesByOutbound()["vpn"]); got != maxSelectiveRoutes {
		t.Fatalf("retained routes = %d, want %d", got, maxSelectiveRoutes)
	}
	if got := acc.Dropped(); got != extra {
		t.Fatalf("dropped = %d, want %d", got, extra)
	}
	// A duplicate of a retained address is a dedupe hit, never a budget drop.
	acc.Add("vpn", "10.0.0.0/32")
	if got := acc.Dropped(); got != extra {
		t.Fatalf("duplicate counted as drop: %d, want %d", got, extra)
	}
}

// TestRouteAccumulator_AtCapNewOutboundLeavesNoEmptySet is the regression test
// for the empty-rule bug: with the budget exhausted, the FIRST address for a
// brand-new outbound must not leave an empty per-outbound set behind — an
// empty set renders an empty ip_cidr list, and buildSelectiveIPRules would
// marshal a condition-less {"action":"route","outbound":X} rule that sing-box
// rejects (reload failure) or treats as match-all.
func TestRouteAccumulator_AtCapNewOutboundLeavesNoEmptySet(t *testing.T) {
	acc := NewRouteAccumulator()
	for i := 0; i < maxSelectiveRoutes; i++ {
		acc.Add("vpn", fmt.Sprintf("%d.%d.%d.%d/32", 10+(i>>24), (i>>16)&0xff, (i>>8)&0xff, i&0xff))
	}
	acc.Add("late-outbound", "9.9.9.9/32") // budget-dropped
	if got := acc.Dropped(); got != 1 {
		t.Fatalf("dropped = %d, want 1", got)
	}
	if _, ok := acc.RulesByOutbound()["late-outbound"]; ok {
		t.Fatal("late-outbound must be absent from RulesByOutbound (empty ip_cidr list would be condition-less)")
	}
	if _, ok := acc.AddrsByOutbound()["late-outbound"]; ok {
		t.Fatal("late-outbound must be absent from AddrsByOutbound")
	}
}
