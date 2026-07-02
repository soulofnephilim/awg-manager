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

// ResolveDomainQueriesStream resolves matchers concurrently (same parallelism as
// ResolveDomainQueries) but streams each /32 to onIP instead of retaining all IPs.
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

	// Probe budget: with a handful of hand-written suffix rules the full
	// subdomain expansion is worth it; a geosite-scale batch gets the
	// minimal set or DNS query volume explodes (matchers × probes ×
	// servers × rounds).
	fullProbes := suffixProbesForBatch(queries)

	sem := make(chan struct{}, maxConcurrentResolves)
	var wg sync.WaitGroup
	var doneCount atomic.Int32
	total := len(queries)

	for _, q := range queries {
		wg.Add(1)
		go func(query DomainQuery) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			rec := ResolveOneQueryStream(ctx, query, dnsServers, func(cidr string) {
				if sink.OnIP != nil {
					sink.OnIP(query, cidr)
				}
			}, hostProgressFn, fullProbes)

			if sink.OnRecord != nil {
				sink.OnRecord(query, rec)
			}
			if progressFn != nil {
				n := int(doneCount.Add(1))
				progressFn(n, total, query.Matcher)
			}
		}(q)
	}
	wg.Wait()
}

// ResolveOneQueryStream resolves a single matcher, invoking onIP for each
// discovered /32 (or wider static CIDR passed through) without retaining
// the full IP list in the returned record. fullProbes selects the suffix
// subdomain expansion mode — batch callers derive it via suffixProbesForBatch.
func ResolveOneQueryStream(
	ctx context.Context,
	query DomainQuery,
	dnsServers []string,
	onIP func(cidr string),
	hostProgressFn ResolveHostProgressFn,
	fullProbes bool,
) MatcherRecord {
	ipsSink := func(list []string) {
		if onIP == nil {
			return
		}
		for _, ip := range list {
			onIP(ip)
		}
	}
	full := resolveOneQuery(ctx, query, dnsServers, nil, hostProgressFn, fullProbes)
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
