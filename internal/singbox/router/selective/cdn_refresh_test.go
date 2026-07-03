package selective

import (
	"context"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCDNQueriesFromConfigDir(t *testing.T) {
	dir := t.TempDir()
	w, err := newSnapshotWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = w.WriteRecord(DomainMatcherRecord{Matcher: "cdn.example.com", Kind: "suffix", CDN: true})
	_ = w.WriteRecord(DomainMatcherRecord{Matcher: "plain.example.com", Kind: "suffix"})
	_ = w.WriteRecord(DomainMatcherRecord{Matcher: "broken.example.com", Kind: "suffix", CDN: true, Error: "no A"})
	if err := w.CloseAndCommit(SnapshotSummary{RebuiltAt: "x", EntryCount: 1}); err != nil {
		t.Fatal(err)
	}
	got, err := CDNQueriesFromConfigDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Matcher != "cdn.example.com" {
		t.Fatalf("expected only CDN matcher without error, got %v", got)
	}
}

func addrSet(ips ...string) map[netip.Addr]struct{} {
	out := make(map[netip.Addr]struct{}, len(ips))
	for _, ip := range ips {
		out[netip.MustParseAddr(ip)] = struct{}{}
	}
	return out
}

func TestMergeRoutesLocked(t *testing.T) {
	b := &Builder{routes: map[string]map[netip.Addr]struct{}{
		"vpn": addrSet("1.1.1.1"),
	}}

	// New IP for an existing outbound + a brand-new outbound → both merged.
	// Every fresh address (including the already-resident 1.1.1.1) is accepted
	// for the ipset.
	n, accepted, dropped := b.mergeRoutesLocked(map[string]map[netip.Addr]struct{}{
		"vpn":   addrSet("1.1.1.1", "2.2.2.2"),
		"proxy": addrSet("3.3.3.3"),
	})
	if n != 2 || dropped != 0 {
		t.Fatalf("merged = %d dropped = %d, want 2/0", n, dropped)
	}
	if len(accepted) != 3 {
		t.Fatalf("accepted = %v, want 3 entries", accepted)
	}
	slices.Sort(accepted)
	if want := []string{"1.1.1.1/32", "2.2.2.2/32", "3.3.3.3/32"}; !slices.Equal(accepted, want) {
		t.Fatalf("accepted = %v, want %v", accepted, want)
	}
	if got := len(b.routes["vpn"]); got != 2 {
		t.Fatalf("vpn routes = %d, want 2", got)
	}
	if got := len(b.routes["proxy"]); got != 1 {
		t.Fatalf("proxy routes = %d, want 1", got)
	}

	// Rendered output stays sorted "/32" strings for the routes slot.
	rendered := b.LastIPRulesByOutbound()
	if got := rendered["vpn"]; len(got) != 2 || got[0] != "1.1.1.1/32" || got[1] != "2.2.2.2/32" {
		t.Fatalf("rendered vpn routes = %v", got)
	}

	// Re-merging the same set is a no-op merge — the caller must not reload —
	// but the addresses stay accepted for ipset re-adds (-exist).
	n, accepted, dropped = b.mergeRoutesLocked(map[string]map[netip.Addr]struct{}{
		"vpn":   addrSet("2.2.2.2"),
		"proxy": addrSet("3.3.3.3"),
	})
	if n != 0 || dropped != 0 || len(accepted) != 2 {
		t.Fatalf("re-merge = %d dropped = %d accepted = %v, want 0/0/2 entries", n, dropped, accepted)
	}

	// Nil map on a builder with no routes stays nil-safe.
	empty := &Builder{}
	if n, accepted, dropped := empty.mergeRoutesLocked(nil); n != 0 || dropped != 0 || accepted != nil {
		t.Fatalf("nil merge = %d/%v/%d, want zero values", n, accepted, dropped)
	}
}

// routesAtCap builds a resident overlay holding exactly maxSelectiveRoutes
// addresses under one outbound.
func routesAtCap(outbound string) map[string]map[netip.Addr]struct{} {
	set := make(map[netip.Addr]struct{}, maxSelectiveRoutes)
	for i := 0; i < maxSelectiveRoutes; i++ {
		set[netip.AddrFrom4([4]byte{10, byte(i >> 16), byte(i >> 8), byte(i)})] = struct{}{}
	}
	return map[string]map[netip.Addr]struct{}{outbound: set}
}

// TestMergeRoutesLocked_AtCap guards the budget edge: with the overlay full,
// fresh addresses must be rejected (never accepted for the ipset — the PR424
// leak), an already-resident address stays accepted, and a brand-new outbound
// must not leave an empty per-outbound map behind (condition-less rule).
func TestMergeRoutesLocked_AtCap(t *testing.T) {
	b := &Builder{routes: routesAtCap("vpn")}

	resident := netip.AddrFrom4([4]byte{10, 0, 0, 1}) // in routesAtCap
	n, accepted, dropped := b.mergeRoutesLocked(map[string]map[netip.Addr]struct{}{
		"vpn":   {resident: {}, netip.MustParseAddr("2.2.2.2"): {}},
		"proxy": addrSet("3.3.3.3"),
	})
	if n != 0 {
		t.Fatalf("merged = %d, want 0", n)
	}
	if dropped != 2 {
		t.Fatalf("dropped = %d, want 2", dropped)
	}
	if want := []string{"10.0.0.1/32"}; !slices.Equal(accepted, want) {
		t.Fatalf("accepted = %v, want %v (only the already-resident address)", accepted, want)
	}
	if _, ok := b.routes["proxy"]; ok {
		t.Fatal("budget-rejected new outbound must not leave an empty routes map (condition-less rule)")
	}
	if got := b.LastIPRulesByOutbound(); len(got["vpn"]) != maxSelectiveRoutes {
		t.Fatalf("vpn overlay changed size: %d", len(got["vpn"]))
	}
}

// fakeIPSetBinary installs a stub ipset executable that records every
// invocation (argv + stdin) into logPath and restores the real search paths
// on cleanup.
func fakeIPSetBinary(t *testing.T) (logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "ipset.log")
	script := "#!/bin/sh\n" +
		"echo \"argv: $*\" >> \"" + logPath + "\"\n" +
		"cat | sed 's/^/stdin: /' >> \"" + logPath + "\"\n" +
		"exit 0\n"
	bin := filepath.Join(dir, "ipset")
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	orig := ipsetBinaryPaths
	t.Cleanup(func() { ipsetBinaryPaths = orig })
	ipsetBinaryPaths = []string{bin}
	return logPath
}

// TestRefreshCDNMatchers_AtCapAddsNothingToIpset is the regression test for
// the refresh-time PR424 leak: with the resident overlay at
// maxSelectiveRoutes, freshly resolved CDN IPs are budget-rejected — they must
// NOT be added to the live ipset (an ipset entry without an overlay rule is
// intercepted, matches no ip_cidr rule and leaves via route.final=direct) and
// the truncation must be surfaced in the persisted snapshot summary.
func TestRefreshCDNMatchers_AtCapAddsNothingToIpset(t *testing.T) {
	stubResolveOneQuery(t, nil) // 2 deterministic 10.x.y.{1,2}/32 per matcher
	ipsetLog := fakeIPSetBinary(t)

	dir := t.TempDir()
	b := &Builder{
		cfg:     BuilderConfig{ConfigDir: dir},
		routes:  routesAtCap("vpn"),
		summary: &SnapshotSummary{RebuiltAt: "x", EntryCount: maxSelectiveRoutes},
	}

	// stubIPsFor("cdn.example") yields two 10.x.y.z addresses; routesAtCap
	// spans 10.0.0.0–10.0.255.255, so make sure the stub output is NOT
	// already resident (already-resident addrs are legitimately re-added).
	for _, ip := range stubIPsFor("cdn.example") {
		addr, ok := hostRouteAddr(ip)
		if !ok {
			t.Fatalf("stub ip %q not a host route", ip)
		}
		if _, resident := b.routes["vpn"][addr]; resident {
			t.Skipf("stub address %s collides with the pre-filled overlay", ip)
		}
	}

	newRoutes, err := b.RefreshCDNMatchers(context.Background(),
		[]DomainQuery{{Matcher: "cdn.example", Kind: KindDomainSuffix, Outbound: "vpn"}}, nil)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if newRoutes != 0 {
		t.Fatalf("newRoutes = %d, want 0 (budget full)", newRoutes)
	}

	// No `ipset restore` (adds) may have run — only the read-only `list` for
	// EntryCount is allowed.
	if data, err := os.ReadFile(ipsetLog); err == nil {
		s := string(data)
		if strings.Contains(s, "restore") || strings.Contains(s, "stdin: add ") {
			t.Fatalf("budget-rejected addresses reached the live ipset:\n%s", s)
		}
	}

	// Truncation surfaced: in-memory summary and the persisted meta.
	if got := b.LastSummary(); got == nil || got.TruncatedRoutes != 2 {
		t.Fatalf("in-memory TruncatedRoutes = %+v, want 2", got)
	}
	if persisted := readSnapshotSummary(dir); persisted == nil || persisted.TruncatedRoutes != 2 {
		t.Fatalf("persisted TruncatedRoutes = %+v, want 2", persisted)
	}

	// The rejected outbound situation must not corrupt the overlay: still
	// exactly the resident cap under "vpn", no empty maps.
	rendered := b.LastIPRulesByOutbound()
	if len(rendered) != 1 || len(rendered["vpn"]) != maxSelectiveRoutes {
		t.Fatalf("overlay changed: outbounds=%d vpn=%d", len(rendered), len(rendered["vpn"]))
	}
}

// TestRefreshCDNMatchers_UnderBudgetAddsAcceptedToIpset is the positive
// counterpart: below the budget, freshly resolved IPs are merged into the
// overlay AND added to the live ipset.
func TestRefreshCDNMatchers_UnderBudgetAddsAcceptedToIpset(t *testing.T) {
	stubResolveOneQuery(t, nil)
	ipsetLog := fakeIPSetBinary(t)

	dir := t.TempDir()
	b := &Builder{
		cfg:     BuilderConfig{ConfigDir: dir},
		summary: &SnapshotSummary{RebuiltAt: "x"},
	}

	newRoutes, err := b.RefreshCDNMatchers(context.Background(),
		[]DomainQuery{{Matcher: "cdn.example", Kind: KindDomainSuffix, Outbound: "vpn"}}, nil)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if newRoutes != 2 {
		t.Fatalf("newRoutes = %d, want 2", newRoutes)
	}
	data, err := os.ReadFile(ipsetLog)
	if err != nil {
		t.Fatalf("ipset log: %v", err)
	}
	for _, ip := range stubIPsFor("cdn.example") {
		if !strings.Contains(string(data), "stdin: add "+SetName+" "+strings.TrimSuffix(ip, "/32")) {
			t.Fatalf("accepted address %s missing from ipset restore input:\n%s", ip, data)
		}
	}
	if got := b.LastSummary(); got == nil || got.TruncatedRoutes != 0 {
		t.Fatalf("TruncatedRoutes = %+v, want 0", got)
	}
	if got := b.LastIPRulesByOutbound()["vpn"]; len(got) != 2 {
		t.Fatalf("overlay vpn routes = %v, want 2", got)
	}
}

func TestReadSnapshotMatchersPagination(t *testing.T) {
	dir := t.TempDir()
	w, err := newSnapshotWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		_ = w.WriteRecord(DomainMatcherRecord{Matcher: "host.example", Kind: "domain"})
	}
	if err := w.CloseAndCommit(SnapshotSummary{EntryCount: 5}); err != nil {
		t.Fatal(err)
	}
	page, total, err := ReadSnapshotMatchers(dir, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 || len(page) != 2 {
		t.Fatalf("total=%d page=%d", total, len(page))
	}
	if _, err := os.Stat(filepath.Join(dir, snapshotMetaFile)); err != nil {
		t.Fatalf("meta file missing: %v", err)
	}
}
