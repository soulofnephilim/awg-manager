package router

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/hoaxisr/awg-manager/internal/singbox/router/selective"
	"github.com/hoaxisr/awg-manager/internal/storage"
	sysexec "github.com/hoaxisr/awg-manager/internal/sys/exec"
)

// routeFinalAllowsSelectiveBypass reports whether route.final is compatible
// with selective ipset bypass. Empty final is treated as direct (same as UI).
func routeFinalAllowsSelectiveBypass(final string) bool {
	return final == "" || final == "direct"
}

func (s *ServiceImpl) validateSelectiveBypassSettings(ctx context.Context, sr storage.SingboxRouterSettings) error {
	if !sr.SelectiveBypass {
		return nil
	}
	if sr.RoutingMode != "tproxy" {
		return fmt.Errorf("selectiveBypass only applies to routingMode %q", "tproxy")
	}
	cfg, err := s.loadRouterConfig()
	if err != nil {
		return fmt.Errorf("selectiveBypass: load router config: %w", err)
	}
	if !routeFinalAllowsSelectiveBypass(cfg.Route.Final) {
		return fmt.Errorf(
			"selectiveBypass requires route.final %q (current %q): catch-all proxy routing sends all traffic through sing-box and conflicts with selective ipset bypass",
			"direct", cfg.Route.Final,
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

// rulesHash returns a short SHA-256 hex digest over the JSON serialisation
// of the route rules and rule sets. Used to detect whether a reconcile
// needs to trigger an ipset rebuild.
func rulesHash(rules []Rule, ruleSets []RuleSet) string {
	h := sha256.New()
	enc := json.NewEncoder(h)
	_ = enc.Encode(rules)
	_ = enc.Encode(ruleSets)
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// triggerSelectiveRebuild triggers a non-blocking ipset rebuild when
// selective-bypass is enabled and the builder is wired. Errors are
// logged but do not fail the reconcile — a stale ipset is better than
// breaking connectivity altogether.
//
// Note: uses context.Background() so the rebuild is NOT cancelled when the
// parent reconcile/enable context finishes. The rebuild can outlive the
// reconcile call (DNS resolution, ipset populate) and must not be cancelled
// mid-way — a partial ipset is worse than none.
func (s *ServiceImpl) triggerSelectiveRebuild(_ context.Context) {
	if s.deps.SelectiveBuilder == nil {
		return
	}
	go func() {
		if err := s.deps.SelectiveBuilder.Rebuild(context.Background()); err != nil {
			s.appLog.Warn("selective-rebuild", "", fmt.Sprintf("ipset rebuild failed: %v", err))
		}
	}()
}

// ensureSelectiveSetExists creates the AWGM-SELECTIVE ipset (empty) if it
// does not already exist. Called before IPTables.Install when SelectiveBypass
// is true so that iptables-restore never references a non-existent set.
// Returns nil when ipset is not installed — the Install call will fail anyway
// with a meaningful error from the xt_set module check.
func ensureSelectiveSetExists(ctx context.Context) error {
	// Try to load xt_set kernel module before creating the set — without it
	// iptables -m set rules will be rejected at COMMIT time.
	if err := selective.EnsureXtSetModule(ctx); err != nil {
		// soft-fail: module may be built-in even without .ko
		_ = err
	}

	bin := selectiveIPSetBinary()
	if bin == "" {
		return nil // ipset not installed — let IPTables.Install surface the real error
	}
	out, err := runIPSet(ctx, bin, "create", selectiveSetName, "hash:net",
		"maxelem", "262144", "family", "inet")
	if err != nil {
		// "set with the same name already exists" → idempotent, not an error
		if containsAny(out, "already exists") {
			return nil
		}
		return fmt.Errorf("ipset create %s: %w (output: %s)", selectiveSetName, err, out)
	}
	return nil
}

// destroySelectiveSet is a thin wrapper that calls ipset destroy without
// importing the selective sub-package into service.go. It uses the
// selectiveSetName constant already defined in iptables.go.
func destroySelectiveSet(ctx context.Context) error {
	bin := selectiveIPSetBinary()
	if bin == "" {
		return nil // ipset not installed — nothing to destroy
	}
	res, err := runIPSet(ctx, bin, "destroy", selectiveSetName)
	if err != nil {
		if res != "" && (containsAny(res, "does not exist", "not found")) {
			return nil
		}
		return fmt.Errorf("ipset destroy %s: %w", selectiveSetName, err)
	}
	return nil
}

// selectiveIPSetBinary returns the path to ipset or "".
// Mirrors selective.IPSetBinary without importing the sub-package.
func selectiveIPSetBinary() string {
	for _, p := range []string{"/opt/sbin/ipset", "/usr/sbin/ipset", "/sbin/ipset"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// runIPSet executes ipset with the given args and returns (combined output, error).
func runIPSet(ctx context.Context, bin string, args ...string) (string, error) {
	res, err := sysexec.Run(ctx, bin, args...)
	out := ""
	if res != nil {
		out = res.Stdout + res.Stderr
	}
	return out, err
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
