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
func (b *Builder) RefreshCDNMatchers(
	ctx context.Context,
	queries []DomainQuery,
	singboxDNS []SingboxDNSServer,
) error {
	if len(queries) == 0 {
		return nil
	}
	if !heavyop.Default.TryLock() {
		if b.cfg.Log != nil {
			b.cfg.Log.Info("selective-cdn-refresh", "", "skipped: sing-box apply or rebuild in progress")
		}
		return nil
	}
	defer heavyop.Default.Unlock()
	if !b.opMu.TryLock() {
		if b.cfg.Log != nil {
			b.cfg.Log.Info("selective-cdn-refresh", "", "skipped: full rebuild in progress")
		}
		return nil
	}
	defer b.opMu.Unlock()

	if err := CreateSet(ctx); err != nil {
		return fmt.Errorf("selective cdn refresh: create set: %w", err)
	}

	dnsServers := BuildDNSServers(ctx, singboxDNS, b.cfg.DNSSource)
	var added int

	for _, q := range queries {
		var chunk []string
		ResolveOneQueryStream(ctx, q, dnsServers, func(cidr string) {
			chunk = append(chunk, cidr)
			if len(chunk) >= IpsetChunkSize {
				if err := ChunkedAddLive(ctx, chunk); err == nil {
					added += len(chunk)
				}
				chunk = chunk[:0]
			}
		}, nil)
		if len(chunk) > 0 {
			if err := ChunkedAddLive(ctx, chunk); err != nil {
				if b.cfg.Log != nil {
					b.cfg.Log.Warn("selective-cdn-refresh", q.Matcher, err.Error())
				}
			} else {
				added += len(chunk)
			}
		}
	}

	b.mu.Lock()
	if b.summary != nil {
		cp := *b.summary
		cp.EntryCount = EntryCount(ctx)
		cp.LastCDNRefresh = time.Now().UTC().Format(time.RFC3339)
		b.summary = &cp
		writeSnapshotMeta(b.cfg.ConfigDir, cp)
	}
	b.mu.Unlock()

	if b.cfg.Log != nil {
		b.cfg.Log.Info("selective-cdn-refresh", "",
			fmt.Sprintf("CDN refresh: %d matchers, ~%d adds (ipset now %d entries)",
				len(queries), added, EntryCount(ctx)))
	}
	b.publishStatus(true, EntryCount(ctx))
	return nil
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
