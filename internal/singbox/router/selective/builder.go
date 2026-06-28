package selective

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hoaxisr/awg-manager/internal/logging"
	"github.com/hoaxisr/awg-manager/internal/singbox/heavyop"
)

// Phase describes the current stage of an ipset rebuild.
type Phase string

const (
	PhaseCollecting Phase = "collecting"
	PhaseResolving  Phase = "resolving"
	PhasePopulating Phase = "populating"
	PhaseDone       Phase = "done"
	PhaseError      Phase = "error"
)

// Progress carries rebuild state for SSE. Intentionally omits IP lists.
type Progress struct {
	Phase   Phase  `json:"phase"`
	Message string `json:"message"`
	Current int    `json:"current"`
	Total   int    `json:"total"`
	// Matcher is the domain rule currently being resolved (resolving phase).
	Matcher string `json:"matcher,omitempty"`
	// QueryHost is the expanded hostname being queried.
	QueryHost string `json:"queryHost,omitempty"`
}

// ProgressFn is called with progress updates during a Rebuild. May be nil.
type ProgressFn func(Progress)

// EventPublisher is the narrow interface for publishing SSE events.
type EventPublisher interface {
	Publish(eventType string, data any)
}

// BuilderConfig holds the external dependencies for Builder.
type BuilderConfig struct {
	ConfigDir string
	DNSSource DNSServerSource
	Log       *logging.ScopedLogger
	Bus       EventPublisher
	Geo       GeoPaths
	// OpenRuleSetJSON returns a streamable JSON path for remote/local rule sets.
	OpenRuleSetJSON RuleSetJSONOpener
}

// Builder orchestrates streaming collection, DNS resolve, and ipset populate.
type Builder struct {
	cfg BuilderConfig

	mu          sync.Mutex
	opMu        sync.Mutex
	lastRebuild time.Time
	lastError   string
	summary     *SnapshotSummary
	routes      map[string][]string
}

// NewBuilder constructs a Builder with the given configuration.
func NewBuilder(cfg BuilderConfig) *Builder {
	RemoveLegacySnapshotJSON(cfg.ConfigDir)
	b := &Builder{cfg: cfg}
	if ts := readLastRebuild(cfg.ConfigDir); !ts.IsZero() {
		b.lastRebuild = ts
	}
	b.summary = readSnapshotSummary(cfg.ConfigDir)
	return b
}

func (b *Builder) LastRebuild() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.lastRebuild.IsZero() {
		return ""
	}
	return b.lastRebuild.UTC().Format(time.RFC3339)
}

func (b *Builder) LastError() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastError
}

// LastSnapshot returns summary metadata only (no matcher list in RAM).
func (b *Builder) LastSnapshot() *RebuildSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.summary == nil {
		return nil
	}
	return summaryToLegacyAPI(b.summary)
}

// LastSummary returns the in-memory rebuild summary.
func (b *Builder) LastSummary() *SnapshotSummary {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.summary == nil {
		return nil
	}
	cp := *b.summary
	return &cp
}

// LastIPRulesByOutbound returns /32 overlay rules for 19-selective-routes.json.
func (b *Builder) LastIPRulesByOutbound() map[string][]string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.routes) == 0 {
		return nil
	}
	out := make(map[string][]string, len(b.routes))
	for k, v := range b.routes {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func summaryToLegacyAPI(s *SnapshotSummary) *RebuildSnapshot {
	if s == nil {
		return nil
	}
	return &RebuildSnapshot{
		RebuiltAt:          s.RebuiltAt,
		EntryCount:         s.EntryCount,
		StaticCIDRCount:    s.StaticCIDRCount,
		DomainMatcherCount: s.DomainMatcherCount,
		LastCDNRefresh:     s.LastCDNRefresh,
		StaticCIDRs:        []string{},
		DomainResults:      []DomainResolveResult{},
	}
}

// Rebuild streams matchers → DNS → staging ipset → atomic swap.
func (b *Builder) Rebuild(
	ctx context.Context,
	rules []RuleJSON,
	ruleSetRefs []RuleSetRef,
	singboxDNS []SingboxDNSServer,
	fn ProgressFn,
) error {
	heavyop.Default.Lock()
	defer heavyop.Default.Unlock()
	b.opMu.Lock()
	defer b.opMu.Unlock()

	emit := func(p Progress) {
		if fn != nil {
			fn(p)
		}
		if b.cfg.Bus != nil {
			b.cfg.Bus.Publish("singbox-router:selective-progress", p)
		}
	}

	emit(Progress{Phase: PhaseCollecting, Message: "Сбор IP-адресов и доменов из правил маршрутизации…"})

	if err := CreateSet(ctx); err != nil {
		return b.fail(ctx, emit, fmt.Errorf("selective: create live set: %w", err))
	}
	if err := EnsureStagingSet(ctx); err != nil {
		return b.fail(ctx, emit, fmt.Errorf("selective: create staging set: %w", err))
	}
	if err := FlushStagingSet(ctx); err != nil {
		return b.fail(ctx, emit, fmt.Errorf("selective: flush staging: %w", err))
	}

	snapW, err := newSnapshotWriter(b.cfg.ConfigDir)
	if err != nil {
		return b.fail(ctx, emit, fmt.Errorf("selective: snapshot writer: %w", err))
	}
	defer snapW.Abort()

	routes := NewRouteAccumulator()
	ctQueue := newConntrackQueue()
	var stagingChunk []string
	var stagingMu sync.Mutex
	flushStaging := func() error {
		stagingMu.Lock()
		defer stagingMu.Unlock()
		if len(stagingChunk) == 0 {
			return nil
		}
		chunk := stagingChunk
		stagingChunk = nil
		return ChunkedAddStaging(ctx, chunk)
	}
	addStaging := func(cidr string) error {
		cidr = normalizeEntry(cidr)
		if cidr == "" {
			return nil
		}
		stagingMu.Lock()
		stagingChunk = append(stagingChunk, cidr)
		if len(stagingChunk) < IpsetChunkSize {
			stagingMu.Unlock()
			return nil
		}
		chunk := stagingChunk
		stagingChunk = nil
		stagingMu.Unlock()
		return ChunkedAddStaging(ctx, chunk)
	}

	var staticCount atomic.Int32
	var domainCount atomic.Int32
	dnsServers := BuildDNSServers(ctx, singboxDNS, b.cfg.DNSSource)

	var queries []DomainQuery
	sink := CollectSink{
		OnStaticCIDR: func(cidr string) error {
			staticCount.Add(1)
			return addStaging(cidr)
		},
		OnDomainQuery: func(q DomainQuery) error {
			queries = append(queries, q)
			return nil
		},
	}

	collectErrs := StreamCollectFromRules(ctx, rules, ruleSetRefs, b.cfg.Geo, b.cfg.OpenRuleSetJSON, sink)
	for _, e := range collectErrs {
		if b.cfg.Log != nil {
			b.cfg.Log.Warn("selective-rebuild", "collect", e.Error())
		}
	}

	totalDomains := len(queries)
	emit(Progress{
		Phase:   PhaseResolving,
		Message: fmt.Sprintf("DNS-резолв %d доменных правил…", totalDomains),
		Total:   totalDomains,
	})

	ResolveDomainQueriesStream(ctx, queries, dnsServers, DomainQueryStreamSink{
		OnIP: func(q DomainQuery, cidr string) {
			_ = addStaging(cidr)
			routes.Add(q.Outbound, cidr)
			if conntrackDestArg(cidr) != "" {
				ctQueue.Add(cidr)
			}
		},
		OnRecord: func(q DomainQuery, rec MatcherRecord) {
			if rec.Error != "" && b.cfg.Log != nil {
				b.cfg.Log.Warn("selective-rebuild", q.Matcher, rec.Error)
			}
			_ = snapW.WriteRecord(DomainMatcherRecord{
				Matcher:    rec.Matcher,
				Kind:       string(rec.Kind),
				QueryHosts: rec.QueryHosts,
				CDN:        rec.CDN,
				Outbound:   rec.Outbound,
				Error:      rec.Error,
			})
			domainCount.Add(1)
		},
	}, func(done, total int, matcher string) {
		emit(Progress{
			Phase:   PhaseResolving,
			Message: fmt.Sprintf("Резолвинг: %s", matcher),
			Matcher: matcher,
			Current: done,
			Total:   total,
		})
	}, func(matcher, host string, hostIndex, hostTotal int) {
		// Host-level SSE only for small lists — avoid flooding during bulk geosite.
		if totalDomains > 200 {
			return
		}
		emit(Progress{
			Phase:     PhaseResolving,
			Message:   fmt.Sprintf("Резолвинг: %s — %s (%d/%d)", matcher, host, hostIndex, hostTotal),
			Matcher:   matcher,
			QueryHost: host,
			Current:   hostIndex,
			Total:     hostTotal,
		})
	})

	emit(Progress{
		Phase:   PhasePopulating,
		Message: "Активация ipset: подмена staging-набора…",
	})

	if err := flushStaging(); err != nil {
		return b.fail(ctx, emit, fmt.Errorf("selective: populate staging: %w", err))
	}

	if err := SwapWithStaging(ctx); err != nil {
		return b.fail(ctx, emit, fmt.Errorf("selective: swap ipset: %w", err))
	}
	_ = FlushStagingSet(ctx)

	entryCount := EntryCount(ctx)
	b.setSuccess()
	b.mu.Lock()
	b.routes = routes.RulesByOutbound()
	b.mu.Unlock()

	summary := SnapshotSummary{
		RebuiltAt:          b.LastRebuild(),
		EntryCount:         entryCount,
		StaticCIDRCount:    int(staticCount.Load()),
		DomainMatcherCount: int(domainCount.Load()),
	}
	if err := snapW.CloseAndCommit(summary); err != nil {
		return b.fail(ctx, emit, fmt.Errorf("selective: commit snapshot: %w", err))
	}
	b.mu.Lock()
	b.summary = &summary
	b.mu.Unlock()

	go b.deferredConntrackFlush(ctQueue.Drain())

	doneMsg := fmt.Sprintf("ipset обновлён. Записей: %d", entryCount)
	if entryCount == 0 {
		doneMsg = "ipset пустой — правил с IP/доменами нет. Весь трафик идёт в WAN мимо sing-box."
	}
	emit(Progress{Phase: PhaseDone, Message: doneMsg, Current: entryCount, Total: entryCount})
	b.publishStatus(true, entryCount)
	return nil
}

func (b *Builder) fail(ctx context.Context, emit func(Progress), err error) error {
	errMsg := err.Error()
	emit(Progress{Phase: PhaseError, Message: errMsg})
	b.setError(errMsg)
	b.publishStatus(false, EntryCount(ctx))
	return err
}

func (b *Builder) deferredConntrackFlush(hosts []string) {
	if len(hosts) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	flushed, available := FlushConntrackForCIDRs(ctx, hosts, func(ip, errMsg string) {
		if b.cfg.Log != nil {
			b.cfg.Log.Warn("selective-rebuild", ip, "conntrack: "+errMsg)
		}
	})
	if b.cfg.Log != nil {
		if available {
			b.cfg.Log.Info("selective-rebuild", "conntrack",
				fmt.Sprintf("deferred flush: %d hosts, %d flows evicted", len(hosts), flushed))
		}
	}
}

func (b *Builder) publishStatus(success bool, entryCount int) {
	if b.cfg.Bus == nil {
		return
	}
	b.cfg.Bus.Publish("singbox-router:selective-status", map[string]any{
		"available":          IsIPSetAvailable(),
		"xtSetAvailable":     IsXtSetAvailable(),
		"conntrackAvailable": IsConntrackAvailable(),
		"enabled":            true,
		"entryCount":         entryCount,
		"lastRebuild":        b.LastRebuild(),
		"lastError":          b.LastError(),
		"snapshot":           b.LastSnapshot(),
	})
}

func (b *Builder) setSuccess() {
	now := time.Now()
	b.mu.Lock()
	b.lastRebuild = now
	b.lastError = ""
	b.mu.Unlock()
	writeLastRebuild(b.cfg.ConfigDir, now)
}

func (b *Builder) setError(msg string) {
	b.mu.Lock()
	b.lastError = msg
	b.mu.Unlock()
}

const lastRebuildFileName = "selective-last-rebuild"

func lastRebuildPath(configDir string) string {
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, lastRebuildFileName)
}

func readLastRebuild(configDir string) time.Time {
	p := lastRebuildPath(configDir)
	if p == "" {
		return time.Time{}
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339, string(data))
	if err != nil {
		return time.Time{}
	}
	return ts
}

func writeLastRebuild(configDir string, t time.Time) {
	p := lastRebuildPath(configDir)
	if p == "" {
		return
	}
	_ = os.WriteFile(p, []byte(t.UTC().Format(time.RFC3339)), 0644)
}
