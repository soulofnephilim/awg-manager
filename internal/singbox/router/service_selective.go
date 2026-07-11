package router

import (
	"context"
	"errors"
	"fmt"

	"github.com/hoaxisr/awg-manager/internal/singbox/router/selective"
	"github.com/hoaxisr/awg-manager/internal/storage"
)

// routeFinalAllowsSelectiveBypass reports whether route.final is compatible
// with selective ipset bypass. Empty final is treated as direct (same as UI).
func routeFinalAllowsSelectiveBypass(final string) bool {
	return final == "" || final == "direct"
}

// errSelectiveIncompatible marks a DEFINITIVE incompatibility between the
// applied settings/config and selective bypass. Only this error justifies
// the reconcile self-heal persistently flipping SelectiveBypass=false; any
// other validation error (config unreadable — transient flash I/O, torn
// state during apply) means "could not check" and must NOT rewrite settings.
var errSelectiveIncompatible = errors.New("selective bypass incompatible")

// validateSelectiveBypassSettings is the user-facing check (UpdateSettings):
// it validates against the EFFECTIVE config — including a staged draft — so
// the UI rejects enabling selective bypass against the route.final the user
// is currently editing.
//
// Вне tproxy селективный перехват СПИТ: fakeip-пути ipset-цепочки не ставят
// (Reconcile и Enable диспатчатся по режиму раньше tproxy-ветки), поэтому
// унаследованный включённый флаг — валидное «спящее» состояние и не должен
// блокировать несвязанные изменения настроек в fakeip (смену TCP/IP-стека
// и т.п., #486). Отклоняется только ПОПЫТКА включения вне tproxy.
func (s *ServiceImpl) validateSelectiveBypassSettings(ctx context.Context, sr storage.SingboxRouterSettings) error {
	if sr.SelectiveBypass && sr.RoutingMode != "tproxy" {
		if s.deps.Settings != nil {
			if prev, err := s.deps.Settings.Load(); err == nil && prev.SingboxRouter.SelectiveBypass {
				return nil // dormant: был включён — остаётся включённым-спящим
			}
		}
		return fmt.Errorf("%w: selectiveBypass можно включить только в режиме tproxy", errSelectiveIncompatible)
	}
	return s.validateSelectiveBypass(sr, s.loadRouterConfig)
}

// validateSelectiveBypassAgainstApplied is the reconcile-side check: it
// judges only the APPLIED config, never the pending draft. A staged (still
// discardable) draft with route.final != direct must not let the self-heal
// flip the persisted setting from under the user.
func (s *ServiceImpl) validateSelectiveBypassAgainstApplied(sr storage.SingboxRouterSettings) error {
	return s.validateSelectiveBypass(sr, s.loadAppliedRouterConfig)
}

func (s *ServiceImpl) validateSelectiveBypass(sr storage.SingboxRouterSettings, load func() (*RouterConfig, error)) error {
	if !sr.SelectiveBypass {
		return nil
	}
	if sr.RoutingMode != "tproxy" {
		return fmt.Errorf("%w: selectiveBypass only applies to routingMode %q", errSelectiveIncompatible, "tproxy")
	}
	cfg, err := load()
	if err != nil {
		return fmt.Errorf("selectiveBypass: load router config: %w", err)
	}
	if !routeFinalAllowsSelectiveBypass(cfg.Route.Final) {
		return fmt.Errorf(
			"%w: selectiveBypass requires route.final %q (current %q): catch-all proxy routing sends all traffic through sing-box and conflicts with selective ipset bypass",
			errSelectiveIncompatible, "direct", cfg.Route.Final,
		)
	}
	return nil
}

// disableSelectiveBypassIfEnabled turns off selectiveBypass in persisted settings
// and reconciles netfilter. No-op when already disabled.
func (s *ServiceImpl) disableSelectiveBypassIfEnabled(ctx context.Context) error {
	if s.deps.Settings == nil {
		return nil
	}
	settings, err := s.deps.Settings.Load()
	if err != nil {
		return err
	}
	if !settings.SingboxRouter.SelectiveBypass {
		return nil
	}
	settings.SingboxRouter.SelectiveBypass = false
	if err := s.deps.Settings.Save(settings); err != nil {
		return err
	}
	s.appLog.Info("selective", "", "selective bypass disabled because route.final is not direct")
	return s.Reconcile(ctx)
}

// SelectiveBuilder is the interface the router service uses to rebuild the
// AWGM-SELECTIVE ipset when selective-bypass is enabled.
// *selective.Builder satisfies it; tests pass a stub.
type SelectiveBuilder interface {
	// Rebuild populates the AWGM-SELECTIVE ipset from the current rules
	// and rule sets. The call is blocking; progress events are delivered
	// via the underlying ProgressFn wired at construction time.
	Rebuild(ctx context.Context) error
	// RefreshCDN incrementally adds newly resolved CDN matcher IPs without
	// flushing ipset or evicting conntrack (safe for background refresh).
	RefreshCDN(ctx context.Context) error
}

// SelectiveDNSSource provides raw /show/dns-proxy bytes for the selective
// domain resolver fallback path. *ndmsquery.DNSProxyStatusStore satisfies it.
type SelectiveDNSSource interface {
	List(ctx context.Context) ([]byte, error)
}

// triggerSelectiveRebuild triggers a non-blocking ipset rebuild when
// selective-bypass is enabled and the builder is wired. Errors are
// logged but do not fail the reconcile — a stale ipset is better than
// breaking connectivity altogether.
//
// Note: uses context.Background() so the rebuild is NOT cancelled when the
// parent reconcile/enable context finishes. The rebuild can outlive the
// reconcile call (DNS resolution, ipset populate) and must not be cancelled
// mid-way — a partial ipset is worse than none. Настенного таймаута нет
// (бывшие 10 минут валили медленную, но идущую пересборку на MIPS-роутере):
// продолжительность ограничивает stall guard внутри билдера
// (selective.WithStallGuard) — отмена при отсутствии прогресса либо по
// абсолютному предохранителю; ожидание heavy-op гейта на старте системы
// (60+ с sing-box cold-start) билдер выдерживает на собственном терпеливом
// таймауте rebuildAcquireTimeout.
func (s *ServiceImpl) triggerSelectiveRebuild(_ context.Context) {
	if s.deps.SelectiveBuilder == nil {
		return
	}
	go func() {
		// Пересборка на MIPS может занимать минуты и меняет поведение
		// bypass — старт и завершение видны в журнале, детали и счётчики
		// живут в selective-панели (SSE-статус билдера).
		s.appLog.Info("selective-rebuild", "", "ipset rebuild started")
		if err := s.deps.SelectiveBuilder.Rebuild(context.Background()); err != nil {
			s.appLog.Warn("selective-rebuild", "", fmt.Sprintf("ipset rebuild failed: %v", err))
			return
		}
		s.appLog.Info("selective-rebuild", "", "ipset rebuild complete")
	}()
}

// ensureSelectiveSetExists creates the AWGM-SELECTIVE ipset (empty) if it
// does not already exist. Called before IPTables.Install when SelectiveBypass
// is true so that iptables-restore never references a non-existent set.
// Returns nil when ipset is not installed — the Install call will fail anyway
// with a meaningful error from the xt_set module check.
//
// Delegates to selective.CreateSet — the ONE place that knows the set's
// create parameters. A drifted local copy (different maxelem/family) would
// make selective's staged `ipset swap` fail with "sets have different header".
func ensureSelectiveSetExists(ctx context.Context) error {
	// Try to load xt_set kernel module before creating the set — without it
	// iptables -m set rules will be rejected at COMMIT time.
	if err := selective.EnsureXtSetModule(ctx); err != nil {
		// soft-fail: module may be built-in even without .ko
		_ = err
	}

	if selective.IPSetBinary() == "" {
		return nil // ipset not installed — let IPTables.Install surface the real error
	}
	return selective.CreateSet(ctx)
}

// destroySelectiveSet removes the AWGM-SELECTIVE ipset. Idempotent.
func destroySelectiveSet(ctx context.Context) error {
	if selective.IPSetBinary() == "" {
		return nil // ipset not installed — nothing to destroy
	}
	return selective.DestroySet(ctx)
}
