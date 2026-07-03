package selective

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
)

// stubResolveOneQuery installs a network-free resolver returning two
// deterministic /32s per matcher and restores the real one on cleanup.
// onCall (optional) observes every resolved query with its probe mode.
func stubResolveOneQuery(t testing.TB, onCall func(q DomainQuery, fullProbes bool)) {
	t.Helper()
	orig := resolveOneQueryFn
	t.Cleanup(func() { resolveOneQueryFn = orig })
	resolveOneQueryFn = func(_ context.Context, query DomainQuery, _ []string, _ func(domain, err string), _ ResolveHostProgressFn, fullProbes bool) DomainResolveResult {
		if onCall != nil {
			onCall(query, fullProbes)
		}
		return DomainResolveResult{
			Matcher:  query.Matcher,
			Kind:     string(query.Kind),
			IPs:      stubIPsFor(query.Matcher),
			Outbound: query.Outbound,
		}
	}
}

// fnv1a64 hashes s with FNV-1a — test-only helper for deriving stable fake
// resolver output (the production CIDR hash dedupe it once served was
// removed: duplicate ipset restore lines are harmless under -exist).
func fnv1a64(s string) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	h := uint64(offset64)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return h
}

// stubIPsFor derives two stable fake host CIDRs from the matcher name so
// equivalence tests can predict resolver output without any network I/O.
func stubIPsFor(matcher string) []string {
	h := fnv1a64(matcher)
	return []string{
		fmt.Sprintf("10.%d.%d.1/32", byte(h>>8), byte(h)),
		fmt.Sprintf("10.%d.%d.2/32", byte(h>>8), byte(h)),
	}
}

// TestResolveDomainQueriesStream_BoundedGoroutines is the regression test for
// the selective-rebuild OOM: the pre-fix implementation launched one goroutine
// per DomainQuery up front (all parked on a semaphore), so a geosite-scale
// list allocated hundreds of thousands of idle 8KB stacks. The fixed worker
// pool must keep the goroutine count flat regardless of query count.
func TestResolveDomainQueriesStream_BoundedGoroutines(t *testing.T) {
	var maxGoroutines atomic.Int64
	stubResolveOneQuery(t, func(DomainQuery, bool) {
		n := int64(runtime.NumGoroutine())
		for {
			cur := maxGoroutines.Load()
			if n <= cur || maxGoroutines.CompareAndSwap(cur, n) {
				break
			}
		}
	})

	const total = 50_000
	queries := make([]DomainQuery, total)
	for i := range queries {
		queries[i] = DomainQuery{Matcher: fmt.Sprintf("m%d.example", i), Kind: KindDomainSuffix, Outbound: "proxy"}
	}

	baseline := int64(runtime.NumGoroutine())
	var records, ips atomic.Int32
	var lastDone atomic.Int32
	ResolveDomainQueriesStream(context.Background(), queries, nil, DomainQueryStreamSink{
		OnIP:     func(DomainQuery, string) { ips.Add(1) },
		OnRecord: func(DomainQuery, MatcherRecord) { records.Add(1) },
	}, func(done, totalArg int, _ string) {
		if int32(done) > lastDone.Load() {
			lastDone.Store(int32(done))
		}
		if totalArg != total {
			t.Errorf("progress total = %d, want %d", totalArg, total)
		}
	}, nil)

	if got := records.Load(); got != total {
		t.Fatalf("records = %d, want %d", got, total)
	}
	if got := ips.Load(); got != 2*total {
		t.Fatalf("streamed IPs = %d, want %d", got, 2*total)
	}
	if got := lastDone.Load(); got != total {
		t.Fatalf("final progress done = %d, want %d", got, total)
	}
	// Pool workers + feeder + test/runtime helpers. Anything near `total`
	// means the goroutine-per-query fan-out is back.
	limit := baseline + maxConcurrentResolves + 16
	if got := maxGoroutines.Load(); got > limit {
		t.Fatalf("goroutines peaked at %d (baseline %d, limit %d) — resolve fan-out is not bounded", got, baseline, limit)
	}
}

// TestResolveDomainQueriesStream_DrainsOnCancel guards the no-leak property:
// with a cancelled context the feeding goroutine must still be drained and
// every query must still produce a record (fast-fail resolves).
func TestResolveDomainQueriesStream_DrainsOnCancel(t *testing.T) {
	stubResolveOneQuery(t, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	const total = 5_000
	queries := make([]DomainQuery, total)
	for i := range queries {
		queries[i] = DomainQuery{Matcher: fmt.Sprintf("c%d.example", i), Kind: KindDomain}
	}
	var records atomic.Int32
	ResolveDomainQueriesStream(ctx, queries, nil, DomainQueryStreamSink{
		OnRecord: func(DomainQuery, MatcherRecord) { records.Add(1) },
	}, nil, nil)
	if got := records.Load(); got != total {
		t.Fatalf("records after cancel = %d, want %d (feed must be drained)", got, total)
	}
}
