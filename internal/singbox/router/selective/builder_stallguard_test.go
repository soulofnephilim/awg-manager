package selective

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// stallTestBuilder собирает Builder с fake-ipset и укороченными порогами
// stall guard'а (поля stallTimeout/stallHardCap — тестовый seam).
func stallTestBuilder(t *testing.T, stall, hardCap time.Duration) *Builder {
	t.Helper()
	fakeIPSetBinary(t)
	return &Builder{
		cfg:          BuilderConfig{ConfigDir: t.TempDir()},
		stallTimeout: stall,
		stallHardCap: hardCap,
	}
}

func suffixRules(n int) []RuleJSON {
	suffixes := make([]string, n)
	for i := range suffixes {
		suffixes[i] = fmt.Sprintf("slow%d.example", i)
	}
	return []RuleJSON{{Action: "route", Outbound: "proxy", DomainSuffix: suffixes}}
}

// TestRebuild_SlowButProgressingSurvivesStallGuard: резолвер медленный
// (суммарно работа заведомо дольше stallTimeout), но каждый матчер
// завершается — пересборка обязана дойти до конца. С прежним настенным
// дедлайном такой прогон упал бы по «timeout».
func TestRebuild_SlowButProgressingSurvivesStallGuard(t *testing.T) {
	const (
		stall   = 250 * time.Millisecond
		perStep = 50 * time.Millisecond
		queries = 60 // 60 / maxConcurrentResolves(10) воркеров × 50мс = ≥300мс > stallTimeout
	)
	b := stallTestBuilder(t, stall, time.Minute)

	stubResolveOneQuery(t, nil)
	resolveOneQueryFn = func(ctx context.Context, query DomainQuery, _ []string, _ func(domain, err string), _ ResolveHostProgressFn, _ bool, _ func()) DomainResolveResult {
		select {
		case <-time.After(perStep):
		case <-ctx.Done():
		}
		return DomainResolveResult{
			Matcher:  query.Matcher,
			Kind:     string(query.Kind),
			IPs:      stubIPsFor(query.Matcher),
			Outbound: query.Outbound,
		}
	}

	var mu sync.Mutex
	var last Progress
	start := time.Now()
	err := b.Rebuild(context.Background(), suffixRules(queries), nil, nil, func(p Progress) {
		mu.Lock()
		last = p
		mu.Unlock()
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("slow-but-progressing rebuild failed: %v", err)
	}
	if elapsed <= stall {
		t.Fatalf("fixture too fast to prove anything: elapsed %s <= stallTimeout %s", elapsed, stall)
	}
	mu.Lock()
	defer mu.Unlock()
	if last.Phase != PhaseDone {
		t.Fatalf("final phase = %s, want done (msg: %s)", last.Phase, last.Message)
	}
}

// TestRebuild_StalledResolverFailsWithClearMessage: резолвер зависает намертво
// без сигналов прогресса — stall guard отменяет прогон, ошибка и терминальный
// PhaseError несут понятное сообщение «нет прогресса» с последней фазой.
func TestRebuild_StalledResolverFailsWithClearMessage(t *testing.T) {
	b := stallTestBuilder(t, 150*time.Millisecond, time.Minute)

	stubResolveOneQuery(t, nil)
	resolveOneQueryFn = func(ctx context.Context, query DomainQuery, _ []string, _ func(domain, err string), _ ResolveHostProgressFn, _ bool, _ func()) DomainResolveResult {
		<-ctx.Done() // висим до отмены stall guard'ом — ни одного touch
		return DomainResolveResult{Matcher: query.Matcher, Kind: string(query.Kind), Outbound: query.Outbound}
	}

	var mu sync.Mutex
	var events []Progress
	err := b.Rebuild(context.Background(), suffixRules(3), nil, nil, func(p Progress) {
		mu.Lock()
		events = append(events, p)
		mu.Unlock()
	})
	if err == nil {
		t.Fatal("stalled rebuild must fail")
	}
	if !errors.Is(err, ErrStalled) {
		t.Fatalf("errors.Is(err, ErrStalled) must hold, got: %v", err)
	}
	if !strings.Contains(err.Error(), "нет прогресса") {
		t.Fatalf("error must name the stall cause, got: %v", err)
	}
	if !strings.Contains(err.Error(), "фаза") {
		t.Fatalf("error must carry last-progress info, got: %v", err)
	}
	if le := b.LastError(); !strings.Contains(le, "нет прогресса") {
		t.Fatalf("LastError = %q, want stall message", le)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 {
		t.Fatal("no progress events emitted")
	}
	final := events[len(events)-1]
	if final.Phase != PhaseError || !strings.Contains(final.Message, "нет прогресса") {
		t.Fatalf("terminal event = %+v, want PhaseError with stall message", final)
	}
}

// TestRebuild_HardCapStopsTouchingZombie: непрерывно «прогрессирующий», но
// никогда не завершающийся резолвер обязан упереться в абсолютный
// предохранитель с собственным сообщением.
func TestRebuild_HardCapStopsTouchingZombie(t *testing.T) {
	b := stallTestBuilder(t, 100*time.Millisecond, 400*time.Millisecond)

	stubResolveOneQuery(t, nil)
	resolveOneQueryFn = func(ctx context.Context, query DomainQuery, _ []string, _ func(domain, err string), _ ResolveHostProgressFn, _ bool, onHostResolved func()) DomainResolveResult {
		for { // зомби: прогресс есть, завершения нет
			select {
			case <-ctx.Done():
				return DomainResolveResult{Matcher: query.Matcher, Kind: string(query.Kind), Outbound: query.Outbound}
			case <-time.After(20 * time.Millisecond):
				if onHostResolved != nil {
					onHostResolved()
				}
			}
		}
	}

	err := b.Rebuild(context.Background(), suffixRules(1), nil, nil, nil)
	if err == nil {
		t.Fatal("hard-capped rebuild must fail")
	}
	if !strings.Contains(err.Error(), "превышен предохранительный лимит") {
		t.Fatalf("error must name the hard cap, got: %v", err)
	}
	if errors.Is(err, ErrStalled) {
		t.Fatalf("hard cap must not be reported as stall: %v", err)
	}
}
