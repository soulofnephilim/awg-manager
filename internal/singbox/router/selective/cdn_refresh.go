package selective

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/hoaxisr/awg-manager/internal/logging"
	"github.com/hoaxisr/awg-manager/internal/singbox/heavyop"
)

// CDNRefreshInterval is how often background CDN matcher refresh runs.
const CDNRefreshInterval = 20 * time.Minute

// CDNQueriesFromConfigDir scans persisted NDJSON for CDN-flagged matchers.
func CDNQueriesFromConfigDir(configDir string) ([]DomainQuery, error) {
	var out []DomainQuery
	err := ForEachCDNMatcher(configDir, func(rec DomainMatcherRecord) error {
		kind := DomainKind(rec.Kind)
		if kind != KindDomain && kind != KindDomainSuffix {
			kind = KindDomainSuffix
		}
		out = append(out, DomainQuery{Matcher: rec.Matcher, Kind: kind, Outbound: rec.Outbound})
		return nil
	})
	return out, err
}

// RefreshCDNMatchers re-resolves CDN matchers and adds new IPs via ipset -exist.
//
// Returns newRoutes — how many /32 host routes were newly merged into the
// routes overlay (LastIPRulesByOutbound). The caller uses it to decide
// whether the routes slot needs a re-sync + sing-box reload; ipset-only
// additions take effect immediately and need neither.
func (b *Builder) RefreshCDNMatchers(
	ctx context.Context,
	queries []DomainQuery,
	singboxDNS []SingboxDNSServer,
) (newRoutes int, err error) {
	if len(queries) == 0 {
		return 0, nil
	}
	if !b.tryAcquireOp() {
		if b.cfg.Log != nil {
			b.cfg.Log.Info("selective-cdn-refresh", "", "skipped: full rebuild in progress")
		}
		return 0, nil
	}
	defer b.releaseOp()

	dnsServers := BuildDNSServers(ctx, singboxDNS, b.cfg.DNSSource)
	fullProbes := fullProbeFlags(queries)
	// Freshly resolved IPs must ALSO reach the routes overlay: the ipset only
	// decides what is intercepted, while 19-selective-routes.json decides
	// where it goes. An IP present in the set but absent from the overlay is
	// intercepted, matches no ip_cidr rule and leaves via route.final=direct —
	// exactly the leak this refresh exists to prevent.
	acc := NewRouteAccumulator()

	// Resolve phase — DNS I/O and in-memory buffering only, NO heavy-op gate:
	// holding the gate across this multi-minute loop made the rebuild API's
	// TryLock pre-check answer every manual rebuild with a misleading 409
	// «применяется конфигурация sing-box».
	for qi, q := range queries {
		ResolveOneQueryStream(ctx, q, dnsServers, func(cidr string) {
			acc.Add(q.Outbound, cidr)
		}, nil, fullProbes[qi], nil)
	}

	// Mutation phase — the heavy-op gate is held ONLY around the overlay merge
	// + ipset writes. The overlay budget check runs FIRST and only addresses it
	// accepts reach ChunkedAddLive: adding resolved IPs to the live set before
	// the merge meant that, with the overlay at maxSelectiveRoutes, the new
	// entries were intercepted but matched no overlay rule and left via
	// route.final=direct — silently recreating the PR424 leak.
	fresh := acc.AddrsByOutbound()
	dropped := acc.Dropped()
	var added, addFailed int
	if len(fresh) > 0 {
		if !heavyop.Default.TryLock() {
			if b.cfg.Log != nil {
				b.cfg.Log.Info("selective-cdn-refresh", "", "skipped ipset update: sing-box apply or rebuild in progress")
			}
			return 0, nil
		}
		err := func() error {
			defer heavyop.Default.Unlock()
			b.mu.Lock()
			merged, accepted, mergeDropped := b.mergeRoutesLocked(fresh)
			b.mu.Unlock()
			newRoutes = merged
			dropped += mergeDropped
			if len(accepted) == 0 {
				return nil
			}
			if err := CreateSet(ctx); err != nil {
				return fmt.Errorf("selective cdn refresh: create set: %w", err)
			}
			// ChunkedAddLive batches internally (IpsetChunkSize per restore).
			if err := ChunkedAddLive(ctx, accepted); err != nil {
				addFailed = len(accepted)
				if b.cfg.Log != nil {
					b.cfg.Log.Warn("selective-cdn-refresh", "", err.Error())
				}
			} else {
				added = len(accepted)
			}
			return nil
		}()
		if err != nil {
			return 0, err
		}
	}

	entries := EntryCount(ctx)

	b.mu.Lock()
	if b.summary != nil && (dropped > 0 || addFailed == 0) {
		cp := *b.summary
		if dropped > 0 {
			// Surface refresh-time truncation the same way a rebuild does: the
			// persisted snapshot summary is what the status API / UI reads.
			// Accumulates across refreshes; the next full rebuild resets it.
			cp.TruncatedRoutes += dropped
		}
		if addFailed == 0 {
			cp.EntryCount = entries
			cp.LastCDNRefresh = time.Now().UTC().Format(time.RFC3339)
		}
		b.summary = &cp
		writeSnapshotMeta(b.cfg.ConfigDir, cp)
	}
	b.mu.Unlock()

	if dropped > 0 && b.cfg.Log != nil {
		b.cfg.Log.Warn("selective-cdn-refresh", "budget",
			fmt.Sprintf("overlay routes truncated: limit %d reached, %d dropped (not added to ipset)", maxSelectiveRoutes, dropped))
	}
	if b.cfg.Log != nil {
		b.cfg.Log.Info("selective-cdn-refresh", "",
			fmt.Sprintf("CDN refresh: %d matchers, ~%d adds, %d failed, %d new routes, %d budget-dropped (ipset now %d entries)",
				len(queries), added, addFailed, newRoutes, dropped, entries))
	}
	if addFailed > 0 {
		b.publishStatus(false, entries)
		return newRoutes, fmt.Errorf("selective cdn refresh: %d of %d ipset adds failed", addFailed, added+addFailed)
	}
	b.publishStatus(true, entries)
	return newRoutes, nil
}

// mergeRoutesLocked merges freshly resolved per-outbound host routes into
// b.routes under the maxSelectiveRoutes budget — periodic CDN refreshes must
// not grow the resident overlay unbounded. Caller holds b.mu.
//
// Returns:
//   - merged: how many addresses were newly added to the overlay (the caller
//     re-syncs the routes slot only when > 0);
//   - accepted: every fresh address that IS present in the overlay after the
//     merge (newly added or already resident), rendered as "/32" CIDRs. These
//     are the ONLY addresses the caller may add to the live ipset — an ipset
//     entry without a matching overlay rule is intercepted, matches no ip_cidr
//     rule and leaves via route.final=direct (the PR424 leak);
//   - dropped: how many fresh addresses the budget rejected.
func (b *Builder) mergeRoutesLocked(fresh map[string]map[netip.Addr]struct{}) (merged int, accepted []string, dropped int) {
	if len(fresh) == 0 {
		return 0, nil, 0
	}
	if b.routes == nil {
		b.routes = make(map[string]map[netip.Addr]struct{}, len(fresh))
	}
	total := 0
	for _, set := range b.routes {
		total += len(set)
	}
	for outbound, set := range fresh {
		dst := b.routes[outbound]
		for addr := range set {
			if dst != nil {
				if _, ok := dst[addr]; ok {
					// Already routed — safe to (re-)add to the ipset.
					accepted = append(accepted, addr.String()+"/32")
					continue
				}
			}
			if total >= maxSelectiveRoutes {
				dropped++
				continue
			}
			if dst == nil {
				// Create the per-outbound map only after the budget check
				// passes — an empty leftover map would render an empty ip_cidr
				// list and buildSelectiveIPRules would emit a condition-less
				// rule (see RouteAccumulator.Add).
				dst = make(map[netip.Addr]struct{}, len(set))
				b.routes[outbound] = dst
			}
			dst[addr] = struct{}{}
			total++
			merged++
			accepted = append(accepted, addr.String()+"/32")
		}
	}
	return merged, accepted, dropped
}

// CDNRefreshLoop runs periodic CDN-only ipset refresh until stop is closed.
type CDNRefreshLoop struct {
	Interval time.Duration
	Enabled  func() bool
	Refresh  func(ctx context.Context) error
	Log      *logging.ScopedLogger
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
}

// StartCDNRefreshLoop launches the background ticker.
func StartCDNRefreshLoop(interval time.Duration, enabled func() bool, refresh func(context.Context) error, log *logging.ScopedLogger) *CDNRefreshLoop {
	if interval <= 0 {
		interval = CDNRefreshInterval
	}
	l := &CDNRefreshLoop{
		Interval: interval,
		Enabled:  enabled,
		Refresh:  refresh,
		Log:      log,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go l.run()
	return l
}

func (l *CDNRefreshLoop) Stop() {
	l.once.Do(func() { close(l.stop) })
	<-l.done
}

func (l *CDNRefreshLoop) run() {
	defer close(l.done)
	ticker := time.NewTicker(l.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.tick()
		case <-l.stop:
			return
		}
	}
}

func (l *CDNRefreshLoop) tick() {
	if l.Enabled != nil && !l.Enabled() {
		return
	}
	if l.Refresh == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := l.Refresh(ctx); err != nil && l.Log != nil {
		l.Log.Warn("selective-cdn-refresh", "", err.Error())
	}
}
