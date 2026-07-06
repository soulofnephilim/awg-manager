package selective

import (
	"context"
	"sync"
	"sync/atomic"
)

// MatcherRecord is the slim DNS outcome for snapshot / SSE (no IP list).
type MatcherRecord struct {
	Matcher    string
	Kind       DomainKind
	QueryHosts []string
	CDN        bool
	Outbound   string
	Error      string
}

// DomainQueryStreamSink receives per-matcher DNS outcomes during parallel resolve.
type DomainQueryStreamSink struct {
	OnIP     func(query DomainQuery, cidr string)
	OnRecord func(query DomainQuery, rec MatcherRecord)
}

// resolveWorkItem is one matcher queued for the resolve worker pool, with the
// probe mode decided by the producer (fullProbeFlags for batch callers, the
// streaming budget in the builder's collect sink).
type resolveWorkItem struct {
	query      DomainQuery
	fullProbes bool
}

// resolveQueueDepth bounds the collect→resolve handoff channel. Collection
// (local file streaming) is far faster than DNS, so without a bound the
// channel would buffer the whole query list and re-create the memory spike
// the streaming pipeline exists to avoid.
const resolveQueueDepth = 256

// resolveOneQueryFn is the resolver seam; tests stub it to exercise the
// pipeline without network I/O.
var resolveOneQueryFn = resolveOneQuery

// ResolveDomainQueriesStream resolves matchers concurrently (fixed pool of
// maxConcurrentResolves workers) and streams each /32 to sink.OnIP instead of
// retaining all IPs.
//
// A fixed worker pool — NOT goroutine-per-query: the previous implementation
// launched len(queries) goroutines up front, all parked on a semaphore. At
// geosite scale (hundreds of thousands of matchers) those idle ~8KB stacks
// alone accounted for gigabytes of total-vm and OOM-killed the daemon on
// 256–512MB routers.
func ResolveDomainQueriesStream(
	ctx context.Context,
	queries []DomainQuery,
	dnsServers []string,
	sink DomainQueryStreamSink,
	progressFn ResolveProgressFn,
	hostProgressFn ResolveHostProgressFn,
) {
	if len(queries) == 0 {
		return
	}

	// Probe budget: the first fullProbeSuffixBudget suffix matchers (the
	// user's hand-written rules — collection emits rules before rule-sets)
	// keep the full subdomain expansion; the geosite-scale tail gets the
	// minimal set or DNS query volume explodes (matchers × probes ×
	// servers × rounds).
	fullProbes := fullProbeFlags(queries)
	total := len(queries)

	work := make(chan resolveWorkItem, resolveQueueDepth)
	go func() {
		defer close(work)
		for i := range queries {
			work <- resolveWorkItem{query: queries[i], fullProbes: fullProbes[i]}
		}
	}()
	resolveWorkers(ctx, work, dnsServers, sink, progressFn, func() int { return total }, hostProgressFn, nil)
}

// resolveWorkers runs the fixed pool of maxConcurrentResolves workers until
// work is closed and drained. Workers keep consuming after ctx cancellation —
// resolveOneQuery fails fast on a dead context and still yields a per-matcher
// record, matching the pre-pool semantics — so the feeding goroutine can
// never leak blocked on send. totalFn supplies the (possibly still growing)
// total for progress reporting; done may briefly exceed a stale total, so the
// reported total is clamped to at least done. onHostResolved (optional)
// пробрасывается в резолвер — сигнал прогресса на каждый завершённый
// DNS-резолв хоста (см. stall guard в builder.RebuildOwnedRun).
func resolveWorkers(
	ctx context.Context,
	work <-chan resolveWorkItem,
	dnsServers []string,
	sink DomainQueryStreamSink,
	progressFn ResolveProgressFn,
	totalFn func() int,
	hostProgressFn ResolveHostProgressFn,
	onHostResolved func(),
) {
	var wg sync.WaitGroup
	var doneCount atomic.Int32
	for w := 0; w < maxConcurrentResolves; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range work {
				query := item.query
				rec := ResolveOneQueryStream(ctx, query, dnsServers, func(cidr string) {
					if sink.OnIP != nil {
						sink.OnIP(query, cidr)
					}
				}, hostProgressFn, item.fullProbes, onHostResolved)

				if sink.OnRecord != nil {
					sink.OnRecord(query, rec)
				}
				if progressFn != nil {
					n := int(doneCount.Add(1))
					total := n
					if totalFn != nil {
						if t := totalFn(); t > total {
							total = t
						}
					}
					progressFn(n, total, query.Matcher)
				}
			}
		}()
	}
	wg.Wait()
}

// ResolveOneQueryStream resolves a single matcher, invoking onIP for each
// discovered /32 (or wider static CIDR passed through) without retaining
// the full IP list in the returned record. fullProbes selects the suffix
// subdomain expansion mode — batch callers derive it via fullProbeFlags.
// onHostResolved (optional) вызывается после каждого завершённого резолва
// хоста — сигнал прогресса для stall guard'а.
func ResolveOneQueryStream(
	ctx context.Context,
	query DomainQuery,
	dnsServers []string,
	onIP func(cidr string),
	hostProgressFn ResolveHostProgressFn,
	fullProbes bool,
	onHostResolved func(),
) MatcherRecord {
	ipsSink := func(list []string) {
		if onIP == nil {
			return
		}
		for _, ip := range list {
			onIP(ip)
		}
	}
	full := resolveOneQueryFn(ctx, query, dnsServers, nil, hostProgressFn, fullProbes, onHostResolved)
	ipsSink(full.IPs)
	return MatcherRecord{
		Matcher:    full.Matcher,
		Kind:       DomainKind(full.Kind),
		QueryHosts: full.QueryHosts,
		CDN:        full.CDN,
		Outbound:   full.Outbound,
		Error:      full.Error,
	}
}

// conntrackQueue collects /32 destinations for deferred flush.
type conntrackQueue struct {
	mu   sync.Mutex
	ips  []string
	seen map[string]struct{}
}

func newConntrackQueue() *conntrackQueue {
	return &conntrackQueue{seen: make(map[string]struct{})}
}

func (q *conntrackQueue) Add(cidr string) {
	cidr = normalizeEntry(cidr)
	if cidr == "" || conntrackDestArg(cidr) == "" {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, ok := q.seen[cidr]; ok {
		return
	}
	q.seen[cidr] = struct{}{}
	q.ips = append(q.ips, cidr)
}

func (q *conntrackQueue) Drain() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := q.ips
	q.ips = nil
	return out
}
