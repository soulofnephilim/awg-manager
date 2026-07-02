package selective

import (
	"context"
	"fmt"
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
	if !heavyop.Default.TryLock() {
		if b.cfg.Log != nil {
			b.cfg.Log.Info("selective-cdn-refresh", "", "skipped: sing-box apply or rebuild in progress")
		}
		return 0, nil
	}
	defer heavyop.Default.Unlock()
	if !b.opMu.TryLock() {
		if b.cfg.Log != nil {
			b.cfg.Log.Info("selective-cdn-refresh", "", "skipped: full rebuild in progress")
		}
		return 0, nil
	}
	defer b.opMu.Unlock()

	if err := CreateSet(ctx); err != nil {
		return 0, fmt.Errorf("selective cdn refresh: create set: %w", err)
	}

	dnsServers := BuildDNSServers(ctx, singboxDNS, b.cfg.DNSSource)
	fullProbes := fullProbeFlags(queries)
	var added, addFailed int
	// Freshly resolved IPs must ALSO reach the routes overlay: the ipset only
	// decides what is intercepted, while 19-selective-routes.json decides
	// where it goes. An IP present in the set but absent from the overlay is
	// intercepted, matches no ip_cidr rule and leaves via route.final=direct —
	// exactly the leak this refresh exists to prevent.
	acc := NewRouteAccumulator()

	for qi, q := range queries {
		var chunk []string
		flush := func() {
			if len(chunk) == 0 {
				return
			}
			if err := ChunkedAddLive(ctx, chunk); err != nil {
				addFailed += len(chunk)
				if b.cfg.Log != nil {
					b.cfg.Log.Warn("selective-cdn-refresh", q.Matcher, err.Error())
				}
			} else {
				added += len(chunk)
			}
			chunk = chunk[:0]
		}
		ResolveOneQueryStream(ctx, q, dnsServers, func(cidr string) {
			chunk = append(chunk, cidr)
			acc.Add(q.Outbound, cidr)
			if len(chunk) >= IpsetChunkSize {
				flush()
			}
		}, nil, fullProbes[qi])
		flush()
	}

	entries := EntryCount(ctx)

	b.mu.Lock()
	newRoutes = b.mergeRoutesLocked(acc.RulesByOutbound())
	if b.summary != nil && addFailed == 0 {
		cp := *b.summary
		cp.EntryCount = entries
		cp.LastCDNRefresh = time.Now().UTC().Format(time.RFC3339)
		b.summary = &cp
		writeSnapshotMeta(b.cfg.ConfigDir, cp)
	}
	b.mu.Unlock()

	if b.cfg.Log != nil {
		b.cfg.Log.Info("selective-cdn-refresh", "",
			fmt.Sprintf("CDN refresh: %d matchers, ~%d adds, %d failed, %d new routes (ipset now %d entries)",
				len(queries), added, addFailed, newRoutes, entries))
	}
	if addFailed > 0 {
		b.publishStatus(false, entries)
		return newRoutes, fmt.Errorf("selective cdn refresh: %d of %d ipset adds failed", addFailed, added+addFailed)
	}
	b.publishStatus(true, entries)
	return newRoutes, nil
}

// mergeRoutesLocked merges freshly resolved per-outbound /32 routes into
// b.routes and reports how many were not present before. Caller holds b.mu.
func (b *Builder) mergeRoutesLocked(fresh map[string][]string) int {
	if len(fresh) == 0 {
		return 0
	}
	if b.routes == nil {
		b.routes = make(map[string][]string, len(fresh))
	}
	merged := 0
	for outbound, cidrs := range fresh {
		existing := make(map[string]struct{}, len(b.routes[outbound]))
		for _, c := range b.routes[outbound] {
			existing[c] = struct{}{}
		}
		for _, c := range cidrs {
			if _, ok := existing[c]; ok {
				continue
			}
			b.routes[outbound] = append(b.routes[outbound], c)
			existing[c] = struct{}{}
			merged++
		}
	}
	return merged
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
