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

	mu sync.Mutex
	// opCh is a 1-slot semaphore serializing Rebuild vs RefreshCDNMatchers.
	// Channel-backed (lazy-init via opOnce so &Builder{} in tests still works)
	// because acquisition must be boundable by a context — see acquireOp.
	opOnce      sync.Once
	opCh        chan struct{}
	rebuilding  atomic.Bool
	lastRebuild time.Time
	lastError   string
	summary     *SnapshotSummary
	routes      map[string][]string
}

func (b *Builder) opSem() chan struct{} {
	b.opOnce.Do(func() { b.opCh = make(chan struct{}, 1) })
	return b.opCh
}

// acquireOp takes the builder op slot, giving up when ctx ends.
func (b *Builder) acquireOp(ctx context.Context) error {
	select {
	case b.opSem() <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// tryAcquireOp takes the builder op slot without blocking.
func (b *Builder) tryAcquireOp() bool {
	select {
	case b.opSem() <- struct{}{}:
		return true
	default:
		return false
	}
}

func (b *Builder) releaseOp() {
	select {
	case <-b.opSem():
	default:
		panic("selective: releaseOp without matching acquire")
	}
}

// Rebuilding reports whether a full Rebuild is currently in flight
// (including waiting on the heavy-op gate).
func (b *Builder) Rebuilding() bool {
	return b.rebuilding.Load()
}

// TryBeginRun marks a rebuild run as in flight, returning false when another
// run already owns the flag. Callers that do expensive pre-work before
// Rebuild proper (the router adapter loads config and materializes rule sets
// for seconds) must mark the run at THEIR entry so both the API handler's
// duplicate check (Rebuilding) and the builder's own dedupe see it from the
// start — otherwise a user POST in that window launches a second full run.
// A successful TryBeginRun must be paired with EndRun and the actual work
// routed through RebuildOwnedRun.
func (b *Builder) TryBeginRun() bool {
	return b.rebuilding.CompareAndSwap(false, true)
}

// EndRun clears the in-flight marker taken by TryBeginRun.
func (b *Builder) EndRun() {
	b.rebuilding.Store(false)
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
// It begins its own run; callers that already marked the run via TryBeginRun
// (the router adapter) must call RebuildOwnedRun instead.
func (b *Builder) Rebuild(
	ctx context.Context,
	rules []RuleJSON,
	ruleSetRefs []RuleSetRef,
	singboxDNS []SingboxDNSServer,
	fn ProgressFn,
) error {
	if !b.TryBeginRun() {
		return b.fail(ctx, b.emitter(fn), fmt.Errorf("selective: %w", ErrBusy))
	}
	defer b.EndRun()
	return b.RebuildOwnedRun(ctx, rules, ruleSetRefs, singboxDNS, fn)
}

// RebuildOwnedRun is Rebuild for a caller that already owns the run marker
// (TryBeginRun returned true). The caller is responsible for EndRun.
func (b *Builder) RebuildOwnedRun(
	ctx context.Context,
	rules []RuleJSON,
	ruleSetRefs []RuleSetRef,
	singboxDNS []SingboxDNSServer,
	fn ProgressFn,
) error {
	emit := b.emitter(fn)

	// Gate acquisition rides on the RUN's own context (the API background run
	// caps it at 10 minutes, the boot auto-rebuild likewise): at boot the
	// orchestrator can legitimately hold the gate 60+ seconds (sing-box cold
	// start readiness floor), and a short hard sub-timeout here made the
	// one-shot boot rebuild give up — leaving a reboot-emptied ipset
	// unpopulated (silent WAN leak). Fast-fail UX for manual rebuilds is
	// provided by the API handler's TryLock pre-check; in-run waiting can be
	// patient.
	if err := heavyop.Default.LockWithContext(ctx); err != nil {
		return b.fail(ctx, emit, fmt.Errorf("selective: %w", ErrBusy))
	}
	defer heavyop.Default.Unlock()
	if err := b.acquireOp(ctx); err != nil {
		return b.fail(ctx, emit, fmt.Errorf("selective: %w", ErrBusy))
	}
	defer b.releaseOp()

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

// emitter wraps the optional per-call ProgressFn with the SSE bus publish.
func (b *Builder) emitter(fn ProgressFn) func(Progress) {
	return func(p Progress) {
		if fn != nil {
			fn(p)
		}
		if b.cfg.Bus != nil {
			b.cfg.Bus.Publish("singbox-router:selective-progress", p)
		}
	}
}

// FailRebuild records err as the last rebuild error and emits the terminal
// progress + status events. For wrappers that fail before Rebuild proper runs
// (config load, rule-set restore) — the UI must still see a final event to
// close its progress view.
func (b *Builder) FailRebuild(ctx context.Context, err error) error {
	return b.fail(ctx, b.emitter(nil), err)
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
