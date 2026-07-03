package selective

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
)

// pipelineFixture builds a rule mix that exercises every collect path:
// hand-written rules (static CIDRs, suffix + exact domains, duplicates) and
// an in-memory inline rule-set large enough to overflow fullProbeSuffixBudget.
func pipelineFixture(bulkDomains int) ([]RuleJSON, []RuleSetRef) {
	domains := make([]interface{}, 0, bulkDomains+2)
	for i := 0; i < bulkDomains; i++ {
		domains = append(domains, fmt.Sprintf("bulk%d.example", i))
	}
	// Duplicates of already-seen matchers must dedupe identically.
	domains = append(domains, "bulk0.example", "hand.example")

	rules := []RuleJSON{
		{Action: "route", Outbound: "proxy", IPCIDR: []string{"1.2.3.0/24", "5.6.7.8", "5.6.7.8/32"},
			DomainSuffix: []string{"hand.example"}, Domain: []string{"exact.example"}},
		{Action: "route", Outbound: "vpn", RuleSet: []string{"bulk"}},
		{Action: "route", Outbound: "direct", DomainSuffix: []string{"skipped.example"}},
	}
	refs := []RuleSetRef{{
		Tag:  "bulk",
		Type: "inline",
		Rules: []map[string]interface{}{
			{"domain_suffix": domains},
			{"ip_cidr": []interface{}{"203.0.113.0/24", "203.0.113.0/24", "198.51.100.7"}},
		},
	}}
	return rules, refs
}

// TestCollectResolveStream_MatchesBatchPipeline is the M2 equivalence golden:
// the streaming collect→resolve pipeline must yield exactly the same ipset
// lines, overlay routes, matcher records and per-matcher probe modes as the
// former two-phase flow (collect all → fullProbeFlags → resolve slice).
func TestCollectResolveStream_MatchesBatchPipeline(t *testing.T) {
	var mu sync.Mutex
	gotProbe := map[string]bool{}
	stubResolveOneQuery(t, func(q DomainQuery, full bool) {
		mu.Lock()
		gotProbe[q.Matcher] = full
		mu.Unlock()
	})

	rules, refs := pipelineFixture(fullProbeSuffixBudget + 40)
	ctx := context.Background()

	// Reference: the pre-streaming shape — collect everything into a slice,
	// then derive probe flags and resolver outputs from the batch.
	wantLines := map[string]struct{}{}
	var batch []DomainQuery
	if _, errs := StreamCollectFromRules(ctx, rules, refs, GeoPaths{}, nil, CollectSink{
		OnStaticCIDR: func(c string) error {
			wantLines[c] = struct{}{}
			return nil
		},
		OnDomainQuery: func(q DomainQuery) error {
			batch = append(batch, q)
			return nil
		},
	}); len(errs) > 0 {
		t.Fatalf("batch collect errors: %v", errs)
	}
	flags := fullProbeFlags(batch)
	wantProbe := map[string]bool{}
	wantRoutes := NewRouteAccumulator()
	wantRecords := len(batch)
	for i, q := range batch {
		wantProbe[q.Matcher] = flags[i]
		for _, ip := range stubIPsFor(q.Matcher) {
			wantLines[normalizeEntry(ip)] = struct{}{}
			wantRoutes.Add(q.Outbound, ip)
		}
	}

	// Under test: the streaming pipeline the builder wires up.
	gotLines := map[string]struct{}{}
	gotRoutes := NewRouteAccumulator()
	var records atomic.Int32
	var queued atomic.Int32
	stats, errs := collectResolveStream(ctx, rules, refs, GeoPaths{}, nil, nil,
		func(cidr string) error {
			mu.Lock()
			gotLines[cidr] = struct{}{}
			mu.Unlock()
			return nil
		},
		DomainQueryStreamSink{
			OnIP: func(q DomainQuery, cidr string) {
				mu.Lock()
				gotLines[normalizeEntry(cidr)] = struct{}{}
				mu.Unlock()
				gotRoutes.Add(q.Outbound, cidr)
			},
			OnRecord: func(DomainQuery, MatcherRecord) { records.Add(1) },
		},
		&queued, nil, nil, nil)
	if len(errs) > 0 {
		t.Fatalf("stream collect errors: %v", errs)
	}
	if stats.DroppedMatchers != 0 {
		t.Fatalf("unexpected matcher truncation: %d", stats.DroppedMatchers)
	}

	if int(records.Load()) != wantRecords {
		t.Fatalf("records = %d, want %d", records.Load(), wantRecords)
	}
	if int(queued.Load()) != wantRecords {
		t.Fatalf("queued = %d, want %d", queued.Load(), wantRecords)
	}
	if !reflect.DeepEqual(sortedKeys(gotLines), sortedKeys(wantLines)) {
		t.Fatalf("ipset lines differ:\n got %v\nwant %v", sortedKeys(gotLines), sortedKeys(wantLines))
	}
	if got, want := gotRoutes.RulesByOutbound(), wantRoutes.RulesByOutbound(); !reflect.DeepEqual(got, want) {
		t.Fatalf("routes differ:\n got %v\nwant %v", got, want)
	}
	if !reflect.DeepEqual(gotProbe, wantProbe) {
		full := 0
		for _, f := range gotProbe {
			if f {
				full++
			}
		}
		t.Fatalf("probe modes differ from batch fullProbeFlags (streamed full=%d)", full)
	}
}

// TestCollectResolveStream_RouteBudgetEdge drives the streaming pipeline past
// maxSelectiveRoutes and pins the budget-edge contract: the accumulator
// retains exactly the budget, reports drops, and never renders an empty
// ip_cidr list — an empty list would let buildSelectiveIPRules marshal a
// condition-less rule that sing-box rejects (reload failure) or match-alls.
func TestCollectResolveStream_RouteBudgetEdge(t *testing.T) {
	stubResolveOneQuery(t, nil)

	const bulk = 80_000
	// Sanity: the stub resolver must yield more distinct addresses than the
	// route budget, or the edge is never reached.
	distinct := map[string]struct{}{}
	for i := 0; i < bulk; i++ {
		for _, ip := range stubIPsFor(fmt.Sprintf("bulk%d.example", i)) {
			distinct[ip] = struct{}{}
		}
	}
	if len(distinct) <= maxSelectiveRoutes {
		t.Fatalf("fixture too small: %d distinct addrs <= budget %d", len(distinct), maxSelectiveRoutes)
	}

	domains := make([]interface{}, bulk)
	for i := range domains {
		domains[i] = fmt.Sprintf("bulk%d.example", i)
	}
	rules := []RuleJSON{{Action: "route", Outbound: "vpn", RuleSet: []string{"bulk"}}}
	refs := []RuleSetRef{{Tag: "bulk", Type: "inline",
		Rules: []map[string]interface{}{{"domain_suffix": domains}}}}

	acc := NewRouteAccumulator()
	var queued atomic.Int32
	_, errs := collectResolveStream(context.Background(), rules, refs, GeoPaths{}, nil, nil,
		func(string) error { return nil },
		DomainQueryStreamSink{
			OnIP:     func(q DomainQuery, cidr string) { acc.Add(q.Outbound, cidr) },
			OnRecord: func(DomainQuery, MatcherRecord) {},
		},
		&queued, nil, nil, nil)
	if len(errs) > 0 {
		t.Fatalf("collect errors: %v", errs)
	}

	total := 0
	for ob, list := range acc.RulesByOutbound() {
		if len(list) == 0 {
			t.Fatalf("outbound %q rendered an empty ip_cidr list", ob)
		}
		total += len(list)
	}
	if total != maxSelectiveRoutes {
		t.Fatalf("retained routes = %d, want exactly %d", total, maxSelectiveRoutes)
	}
	if acc.Dropped() == 0 {
		t.Fatal("expected budget drops past the cap")
	}

	// With the budget exhausted mid-run, the FIRST address for a new outbound
	// must not materialize an empty per-outbound set (F1 regression).
	acc.Add("late-outbound", "9.9.9.9/32")
	if _, ok := acc.RulesByOutbound()["late-outbound"]; ok {
		t.Fatal("late-outbound must be absent from RulesByOutbound at cap")
	}
	if _, ok := acc.AddrsByOutbound()["late-outbound"]; ok {
		t.Fatal("late-outbound must be absent from AddrsByOutbound at cap")
	}
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// BenchmarkCollectResolvePipeline drives the full collect→resolve(stub)→
// accumulate path at geosite scale. Not asserted in CI — the allocs/bytes
// figures feed the PR description. The 80k size stays under the
// maxSelectiveMatchers budget for apples-to-apples comparison with the
// pre-streaming pipeline; 200k also exercises budget truncation.
func BenchmarkCollectResolvePipeline(b *testing.B) {
	for _, size := range []int{80_000, 200_000} {
		b.Run(fmt.Sprintf("%dk", size/1000), func(b *testing.B) {
			var peakGoroutines atomic.Int64
			stubResolveOneQuery(b, func(DomainQuery, bool) {
				n := int64(runtime.NumGoroutine())
				for {
					cur := peakGoroutines.Load()
					if n <= cur || peakGoroutines.CompareAndSwap(cur, n) {
						break
					}
				}
			})
			rules, refs := pipelineFixture(size)
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				acc := NewRouteAccumulator()
				var queued atomic.Int32
				_, _ = collectResolveStream(ctx, rules, refs, GeoPaths{}, nil, nil,
					func(string) error { return nil },
					DomainQueryStreamSink{
						OnIP:     func(q DomainQuery, cidr string) { acc.Add(q.Outbound, cidr) },
						OnRecord: func(DomainQuery, MatcherRecord) {},
					},
					&queued, nil, nil, nil)
			}
			b.ReportMetric(float64(peakGoroutines.Load()), "peak-goroutines")
		})
	}
}
