package selective

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
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
	// routes is the resident /32 overlay (outbound → host addresses). Stored
	// as netip.Addr sets — it stays alive between rebuilds (see
	// LastIPRulesByOutbound / mergeRoutesLocked) and the former
	// map[string][]string of "/32" strings cost several times the bytes.
	routes map[string]map[netip.Addr]struct{}
	// stallTimeout / stallHardCap переопределяют rebuildStallTimeout /
	// rebuildHardCap для тестов (0 — значения по умолчанию).
	stallTimeout time.Duration
	stallHardCap time.Duration
	// lastProgress — последний прогресс текущей пересборки (progressMark);
	// попадает в сообщение об остановке stall guard'ом (stallGuardError).
	lastProgress atomic.Value
	// cancelRun — cancel-cause текущего прогона RebuildOwnedRun (под mu);
	// nil между прогонами. Дёргается из CancelRun по запросу пользователя.
	cancelRun context.CancelCauseFunc
}

// progressMark — компактный снимок последнего прогресса пересборки для
// диагностики stall guard'а.
type progressMark struct {
	phase   Phase
	matcher string
	current int
	total   int
}

func (b *Builder) stallTimeoutOrDefault() time.Duration {
	if b.stallTimeout > 0 {
		return b.stallTimeout
	}
	return rebuildStallTimeout
}

func (b *Builder) stallHardCapOrDefault() time.Duration {
	if b.stallHardCap > 0 {
		return b.stallHardCap
	}
	return rebuildHardCap
}

// lastProgressInfo описывает последний зафиксированный прогресс для
// сообщения об остановке пересборки.
func (b *Builder) lastProgressInfo() string {
	m, _ := b.lastProgress.Load().(progressMark)
	s := "фаза "
	if m.phase == "" {
		s += "запуск"
	} else {
		s += string(m.phase)
	}
	if m.matcher != "" {
		s += ", последний матчер " + m.matcher
	}
	if m.total > 0 {
		s += fmt.Sprintf(", %d/%d", m.current, m.total)
	}
	return s
}

// stallGuardError переписывает ошибку конвейера, вызванную срабатыванием
// stall guard'а, в понятное сообщение с последним прогрессом; обычные ошибки
// проходят без изменений. Причины различаются по context.Cause: ErrStalled —
// нет прогресса, DeadlineExceeded — сработал абсолютный предохранитель.
func (b *Builder) stallGuardError(guardCtx context.Context, err error) error {
	cause := context.Cause(guardCtx)
	switch {
	case errors.Is(cause, ErrCancelledByUser):
		// Явная отмена через CancelRun: причина от родительского runCtx
		// пробрасывается в guard-контекст самим context-пакетом.
		return fmt.Errorf("%w (%s): %w", ErrCancelledByUser, b.lastProgressInfo(), err)
	case errors.Is(cause, ErrStalled):
		// Двойной %w: и причина (ErrStalled — для errors.Is), и ошибка шага,
		// на котором конвейер заметил отмену (для логов).
		return fmt.Errorf("пересборка ipset остановлена: %w %s подряд (%s): %w",
			ErrStalled, formatDurationRu(b.stallTimeoutOrDefault()), b.lastProgressInfo(), err)
	case errors.Is(cause, context.DeadlineExceeded):
		return fmt.Errorf("пересборка ipset остановлена: превышен предохранительный лимит %s (%s): %w",
			formatDurationRu(b.stallHardCapOrDefault()), b.lastProgressInfo(), err)
	}
	return err
}

// CancelRun отменяет текущую пересборку (ограниченный выход из runaway-прогона
// до истечения rebuildHardCap): работает с момента входа в RebuildOwnedRun,
// включая ожидание heavy-op гейта. Безопасный no-op, когда прогона нет —
// возвращает false; true — активный прогон найден и его отмена запрошена.
// reason == nil трактуется как ErrCancelledByUser.
func (b *Builder) CancelRun(reason error) bool {
	if reason == nil {
		reason = ErrCancelledByUser
	}
	b.mu.Lock()
	cancel := b.cancelRun
	b.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel(reason)
	return true
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
// Lists are rendered sorted (see renderHostRoutes) so the marshalled routes
// slot stays byte-stable across identical rebuilds.
func (b *Builder) LastIPRulesByOutbound() map[string][]string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.routes) == 0 {
		return nil
	}
	out := make(map[string][]string, len(b.routes))
	for k, set := range b.routes {
		if len(set) == 0 {
			// Defensive: an empty list becomes a condition-less rule in
			// buildSelectiveIPRules — see RouteAccumulator.Add.
			continue
		}
		out[k] = renderHostRoutes(set)
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
	parentCtx context.Context,
	rules []RuleJSON,
	ruleSetRefs []RuleSetRef,
	singboxDNS []SingboxDNSServer,
	fn ProgressFn,
) error {
	emit := b.emitter(fn)

	// runCtx делает прогон отменяемым по запросу пользователя (CancelRun):
	// регистрируется ДО ожидания heavy-op гейта, чтобы отмена работала и на
	// этом этапе. Причина отмены (ErrCancelledByUser) доезжает до guard-
	// контекста ниже через штатное распространение context.Cause.
	runCtx, cancelRun := context.WithCancelCause(parentCtx)
	defer cancelRun(context.Canceled)
	b.mu.Lock()
	b.cancelRun = cancelRun
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.cancelRun = nil
		b.mu.Unlock()
	}()
	// failAcquire различает пользовательскую отмену и занятый гейт/слот на
	// этапе, когда stall guard ещё не создан.
	failAcquire := func() error {
		if errors.Is(context.Cause(runCtx), ErrCancelledByUser) {
			return b.fail(parentCtx, emit, fmt.Errorf("selective: %w", ErrCancelledByUser))
		}
		return b.fail(parentCtx, emit, fmt.Errorf("selective: %w", ErrBusy))
	}

	// Gate acquisition rides on its own patient timeout (rebuildAcquireTimeout),
	// NOT on the stall guard below — waiting on the gate is not progress and
	// must not trip the guard: at boot the orchestrator can legitimately hold
	// the gate 60+ seconds (sing-box cold start readiness floor), and a short
	// hard sub-timeout here made the one-shot boot rebuild give up — leaving a
	// reboot-emptied ipset unpopulated (silent WAN leak). Fast-fail UX for
	// manual rebuilds is provided by the API handler's TryLock pre-check;
	// in-run waiting can be patient.
	acquireCtx, cancelAcquire := context.WithTimeout(runCtx, rebuildAcquireTimeout)
	defer cancelAcquire()
	if err := heavyop.Default.LockWithContext(acquireCtx); err != nil {
		return failAcquire()
	}
	defer heavyop.Default.Unlock()
	if err := b.acquireOp(acquireCtx); err != nil {
		return failAcquire()
	}
	defer b.releaseOp()

	// Прогресс-ориентированный дедлайн вместо настенного (бывшие 10 минут в
	// API-обработчике и boot-триггере): отмена — только когда touch не
	// вызывался rebuildStallTimeout подряд либо истёк rebuildHardCap.
	// Медленная, но идущая пересборка на MIPS-роутере больше не падает по
	// «timeout». touch дёргают: каждый emit (смена фазы, per-matcher
	// прогресс), sink'и статических CIDR и доменных записей, каждая
	// управляющая ipset-команда и restore-чанк (до и после — runIpsetCtl /
	// addEntriesToSet), завершение каждого DNS-резолва хоста внутри матчера
	// (onHostResolved в resolveOneQuery), а также материализация rule-set'ов
	// и периодический тик geosite/geoip-скана (ProgressTouch через контекст).
	ctx, touch, cancelGuard := WithStallGuard(runCtx, b.stallTimeoutOrDefault(), b.stallHardCapOrDefault())
	defer cancelGuard()
	// touch-хук едет вместе с контекстом в слои, куда прокидывается только
	// ctx: ipset-команды и материализация rule-set'ов в пакете router.
	ctx = ContextWithProgressTouch(ctx, touch)

	b.lastProgress.Store(progressMark{})
	rawEmit := emit
	emit = func(p Progress) {
		// Прогресс фиксируется, только пока guard-контекст жив: emit'ы,
		// догоняющие уже отменённую пересборку (например, смена фазы на
		// populating после stall'а в resolving), не должны переписать
		// lastProgress — иначе сообщение об остановке назовёт не ту фазу.
		if p.Phase != PhaseError && ctx.Err() == nil {
			b.lastProgress.Store(progressMark{phase: p.Phase, matcher: p.Matcher, current: p.Current, total: p.Total})
		}
		touch()
		rawEmit(p)
	}
	// fail завершает пересборку через b.fail, предварительно переписывая
	// ошибку срабатывания stall guard'а в понятное сообщение. EntryCount в
	// b.fail работает на parentCtx — guard-контекст к этому моменту мёртв.
	fail := func(err error) error {
		return b.fail(parentCtx, emit, b.stallGuardError(ctx, err))
	}

	emit(Progress{Phase: PhaseCollecting, Message: "Сбор IP-адресов и доменов из правил маршрутизации…"})

	if err := CreateSet(ctx); err != nil {
		return fail(fmt.Errorf("selective: create live set: %w", err))
	}
	if err := EnsureStagingSet(ctx); err != nil {
		return fail(fmt.Errorf("selective: create staging set: %w", err))
	}
	if err := FlushStagingSet(ctx); err != nil {
		return fail(fmt.Errorf("selective: flush staging: %w", err))
	}

	snapW, err := newSnapshotWriter(b.cfg.ConfigDir)
	if err != nil {
		return fail(fmt.Errorf("selective: snapshot writer: %w", err))
	}
	defer snapW.Abort()

	routes := NewRouteAccumulator()
	ctQueue := newConntrackQueue()
	// commitChunk фиксирует один restore-чанк с сигналами прогресса: «начали
	// операцию» — тоже прогресс (зависание самой команды ловит её
	// exec-таймаут ipsetRestoreTimeout, а не stall guard), фиксация — тем более.
	commitChunk := func(chunk []string) error {
		touch()
		if err := ChunkedAddStaging(ctx, chunk); err != nil {
			return err
		}
		touch()
		return nil
	}
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
		return commitChunk(chunk)
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
		return commitChunk(chunk)
	}

	// staticCount counts collected static-CIDR lines, not unique subnets:
	// in-process CIDR dedupe was removed (unbounded at geoip scale; ipset
	// restore -exist makes duplicates harmless), so a CIDR repeated across
	// rules/rule-sets counts once per sighting. Consumers only need
	// zero-vs-nonzero (NeedsPopulation) or an informational figure (UI);
	// the authoritative entry count is ipset EntryCount.
	var staticCount atomic.Int32
	var domainCount atomic.Int32
	dnsServers := BuildDNSServers(ctx, singboxDNS, b.cfg.DNSSource)

	// Collect and resolve run as ONE streaming pipeline: collected matchers
	// flow through a bounded channel straight into the fixed resolve pool
	// instead of first materializing a geosite-scale []DomainQuery (plus its
	// parallel probe flags). The exact matcher total is therefore only known
	// once collection finishes — so progress stays in the collecting phase
	// (its total still growing) until the collector returns, and only then
	// flips to resolving with a final total. Flipping immediately made the
	// modal mark «Сбор правил» done at t=0 and its bar regress as the moving
	// total grew.
	var queuedDomains atomic.Int32
	var collectDone atomic.Bool
	resolvePhase := func() Phase {
		if collectDone.Load() {
			return PhaseResolving
		}
		return PhaseCollecting
	}
	// Host-level events are suppressed for bulk runs; the latch keeps the
	// decision sticky for the whole run (see hostProgressFn below).
	var suppressHostEvents atomic.Bool
	stats, collectErrs := collectResolveStream(ctx, rules, ruleSetRefs, b.cfg.Geo, b.cfg.OpenRuleSetJSON, dnsServers,
		func(cidr string) error {
			staticCount.Add(1)
			touch() // каждая собранная статическая CIDR-строка — прогресс
			return addStaging(cidr)
		},
		DomainQueryStreamSink{
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
				touch() // завершённый матчер — прогресс
			},
		},
		&queuedDomains,
		func() {
			// Collection finished — the queued count is final from here on.
			collectDone.Store(true)
			emit(Progress{
				Phase:   PhaseResolving,
				Message: "DNS-резолв доменных правил…",
				Total:   int(queuedDomains.Load()),
			})
		},
		func(done, total int, matcher string) {
			emit(Progress{
				Phase:   resolvePhase(),
				Message: fmt.Sprintf("Резолвинг: %s", matcher),
				Matcher: matcher,
				Current: done,
				Total:   total,
			})
		}, func(matcher, host string, hostIndex, hostTotal int) {
			// Начало резолва хоста внутри матчера — прогресс независимо от
			// того, дойдёт ли событие до SSE (подавление ниже касается только
			// частоты событий, не сигнала stall guard'у).
			touch()
			// Host-level SSE only for small lists — avoid flooding during bulk
			// geosite. Once the queued count passes the threshold, suppression
			// latches ON for the rest of the run; and while collection is still
			// streaming (the count could yet grow past the threshold) host
			// events are withheld entirely — so a bulk import cannot leak an
			// initial burst of mismatched-scale events.
			if suppressHostEvents.Load() {
				return
			}
			if queuedDomains.Load() > 200 {
				suppressHostEvents.Store(true)
				return
			}
			if !collectDone.Load() {
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
		},
		// Один матчер легально резолвится минуты (до maxHostsPerMatcher
		// хостов × раунды × 2 с) — прогрессом считается и завершение
		// каждого DNS-резолва хоста внутри матчера.
		touch)
	for _, e := range collectErrs {
		if b.cfg.Log != nil {
			b.cfg.Log.Warn("selective-rebuild", "collect", e.Error())
		}
	}
	routesDropped := routes.Dropped()
	if b.cfg.Log != nil {
		if stats.DroppedMatchers > 0 {
			b.cfg.Log.Warn("selective-rebuild", "budget",
				fmt.Sprintf("matcher list truncated: limit %d exceeded, %d dropped", maxSelectiveMatchers, stats.DroppedMatchers))
		}
		if routesDropped > 0 {
			b.cfg.Log.Warn("selective-rebuild", "budget",
				fmt.Sprintf("overlay routes truncated: limit %d exceeded, %d dropped", maxSelectiveRoutes, routesDropped))
		}
	}

	emit(Progress{
		Phase:   PhasePopulating,
		Message: "Активация ipset: подмена staging-набора…",
	})

	if err := flushStaging(); err != nil {
		return fail(fmt.Errorf("selective: populate staging: %w", err))
	}

	if err := SwapWithStaging(ctx); err != nil {
		return fail(fmt.Errorf("selective: swap ipset: %w", err))
	}
	_ = FlushStagingSet(ctx)

	// После успешного swap пересборка фактически состоялась — счётчик снимаем
	// на parentCtx (guard мог сработать в зазоре после swap и убить ctx) и
	// честно различаем «подтверждённый ноль» и «счётчик прочитать не удалось»:
	// сбой счётчика не должен рождать ложное «ipset пустой — весь трафик в
	// WAN» при только что залитом наборе.
	entryCount, entryCountOK := EntryCountChecked(parentCtx)
	b.setSuccess()
	b.mu.Lock()
	b.routes = routes.AddrsByOutbound()
	b.mu.Unlock()

	summary := SnapshotSummary{
		RebuiltAt:          b.LastRebuild(),
		EntryCount:         entryCount,
		StaticCIDRCount:    int(staticCount.Load()),
		DomainMatcherCount: int(domainCount.Load()),
		TruncatedMatchers:  stats.DroppedMatchers,
		TruncatedRoutes:    routesDropped,
	}
	if err := snapW.CloseAndCommit(summary); err != nil {
		return fail(fmt.Errorf("selective: commit snapshot: %w", err))
	}
	b.mu.Lock()
	b.summary = &summary
	b.mu.Unlock()

	go b.deferredConntrackFlush(ctQueue.Drain())

	doneMsg := fmt.Sprintf("ipset обновлён. Записей: %d", entryCount)
	if !entryCountOK {
		// Swap прошёл, но счётчик снять не удалось — успех с неизвестным
		// количеством, а не сфабрикованный «пустой» набор.
		doneMsg = "ipset обновлён. Записей: н/д (счётчик прочитать не удалось)"
	} else if entryCount == 0 {
		doneMsg = "ipset пустой — правил с IP/доменами нет. Весь трафик идёт в WAN мимо sing-box."
	}
	if stats.DroppedMatchers > 0 {
		doneMsg += fmt.Sprintf("; список усечён: превышен лимит матчеров %d (отброшено %d)", maxSelectiveMatchers, stats.DroppedMatchers)
	}
	if routesDropped > 0 {
		doneMsg += fmt.Sprintf("; маршруты усечены: превышен лимит %d (отброшено %d)", maxSelectiveRoutes, routesDropped)
	}
	emit(Progress{Phase: PhaseDone, Message: doneMsg, Current: entryCount, Total: entryCount})
	b.publishStatus(true, entryCount)
	return nil
}

// collectResolveStream runs the collect stage and the DNS resolve worker pool
// as one streaming pipeline: every collected DomainQuery flows through a
// bounded channel (resolveQueueDepth) straight into the fixed pool of
// maxConcurrentResolves workers, so at no point does a geosite-scale query
// list exist in memory. queued is incremented per enqueued matcher and doubles
// as the moving progress total. onCollected (optional) fires once the collect
// stage has finished — i.e. queued is final — while resolves may still be in
// flight. onHostResolved (optional) fires after every completed DNS host
// resolve inside a matcher — сигнал прогресса для stall guard'а. Returns once
// BOTH stages have finished.
func collectResolveStream(
	ctx context.Context,
	rules []RuleJSON,
	ruleSetRefs []RuleSetRef,
	geo GeoPaths,
	openJSON RuleSetJSONOpener,
	dnsServers []string,
	onStaticCIDR func(cidr string) error,
	resolveSink DomainQueryStreamSink,
	queued *atomic.Int32,
	onCollected func(),
	progressFn ResolveProgressFn,
	hostProgressFn ResolveHostProgressFn,
	onHostResolved func(),
) (CollectStats, []error) {
	work := make(chan resolveWorkItem, resolveQueueDepth)
	resolveDone := make(chan struct{})
	go func() {
		defer close(resolveDone)
		resolveWorkers(ctx, work, dnsServers, resolveSink, progressFn, func() int {
			return int(queued.Load())
		}, hostProgressFn, onHostResolved)
	}()

	// Streaming variant of fullProbeFlags: the first fullProbeSuffixBudget
	// suffix matchers — in collection order, i.e. hand-written rules before
	// rule-sets, exactly like the batch flags — keep the full subdomain
	// expansion; the tail gets minimalSuffixProbes. Deciding at enqueue time
	// in the single-goroutine collector reproduces the batch semantics
	// without knowing the total count up front (exact-domain queries resolve
	// one host anyway and never consume the budget).
	budget := fullProbeSuffixBudget
	sink := CollectSink{
		OnStaticCIDR: onStaticCIDR,
		OnDomainQuery: func(q DomainQuery) error {
			full := q.Kind == KindDomain
			if !full && budget > 0 {
				full = true
				budget--
			}
			queued.Add(1)
			// Workers drain the channel even after cancellation (resolves
			// fail fast on a dead ctx), so this send cannot block forever;
			// the ctx branch just lets collection abort early.
			select {
			case work <- resolveWorkItem{query: q, fullProbes: full}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
	stats, errs := StreamCollectFromRules(ctx, rules, ruleSetRefs, geo, openJSON, sink)
	close(work)
	if onCollected != nil {
		onCollected()
	}
	<-resolveDone
	return stats, errs
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
