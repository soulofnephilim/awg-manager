package router

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hoaxisr/awg-manager/internal/storage"
)

// newReconcileInstalledService builds the minimal ServiceImpl for
// reconcileInstalled tests (same shape as TestReconcile_PolicyMarkChanged_
// Reinstalls): stubbed iptables, no-op preflight, current* seeded so the
// steady state needs no re-Install.
func newReconcileInstalledService(t *testing.T, sb *fakeSingbox) *ServiceImpl {
	t.Helper()
	// singboxReady (tproxy) gates on the inbound-socket probe; stub it "bound"
	// by default so an alive engine reads as ready. Dead-engine cases short-
	// circuit on IsRunning before the probe; the up-but-unbound case overrides
	// this stub to return false.
	stubListeningProbe(t, func() bool { return true })
	ipt := newStubIPTables(func(_ context.Context, _ string) error { return nil })
	return &ServiceImpl{
		deps: Deps{
			Policies:           &fakeAccessPolicyProvider{mark: "0xffffaaa"},
			IPTables:           ipt,
			WANIPCollector:     &fakeWANIPCollector{ips: []string{"203.0.113.207/32"}},
			Singbox:            sb,
			NetfilterPreflight: func(context.Context) error { return nil },
		},
		currentMark:         "0xffffaaa",
		currentWANIPs:       []string{"203.0.113.207/32"},
		netfilterStateKnown: true,
	}
}

var reconcileInstalledSettings = storage.SingboxRouterSettings{
	Enabled:       true,
	PolicyName:    "Policy0",
	WANAutoDetect: true,
}

// Единственный рестарт-авторитет — watchdog (Operator.Reconcile). Мёртвый
// sing-box: tproxy-reconcile НЕ рестартит сам (раньше это был второй авторитет,
// #456). Fail-closed при этом держит blackhole (при снесённых джампах) или
// перехват в мёртвый порт (при целых). Движок поднимет watchdog своим тиком.
func TestReconcileInstalled_DeadSingboxNotRestartedByReconcile(t *testing.T) {
	sb := newTestSingbox(t) // IsRunning по умолчанию false — «процесс мёртв»
	svc := newReconcileInstalledService(t, sb)

	if err := svc.reconcileInstalled(context.Background(), reconcileInstalledSettings); err != nil {
		t.Fatalf("reconcileInstalled: %v", err)
	}
	if sb.startCalls != 0 {
		t.Errorf("reconcile must NOT restart (watchdog is the sole authority): startCalls = %d, want 0", sb.startCalls)
	}
}

// Живой sing-box не трогаем: ни рестарта, ни спавна.
func TestReconcileInstalled_AliveSingboxUntouched(t *testing.T) {
	sb := newTestSingbox(t)
	sb.isRunningFn = func() (bool, int) { return true, 1234 }
	svc := newReconcileInstalledService(t, sb)

	if err := svc.reconcileInstalled(context.Background(), reconcileInstalledSettings); err != nil {
		t.Fatalf("reconcileInstalled: %v", err)
	}
	if sb.startCalls != 0 {
		t.Errorf("alive process must be untouched: start=%d, want 0", sb.startCalls)
	}
}

// GetStatus прокидывает crash-наблюдаемость (#456) из CrashStats в Status.
func TestGetStatus_SurfacesCrashStats(t *testing.T) {
	sb := newTestSingbox(t)
	sb.crashCount = 2
	sb.lastCrashReason = "sing-box убит OOM-killer'ом"
	until := time.Date(2026, 7, 6, 12, 34, 56, 0, time.UTC)
	sb.restartSuppressedUntil = until

	settingsStore := newTestSettingsStore(t, storage.SingboxRouterSettings{
		RoutingMode:   "tproxy",
		PolicyName:    "Policy0",
		WANAutoDetect: true,
	})
	svc := newTestService(t, Deps{
		Settings: settingsStore,
		Singbox:  sb,
		IPTables: errProbeIPTables(),
		Policies: &fakeAccessPolicyProvider{},
	})

	st, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if st.CrashCount != 2 {
		t.Errorf("CrashCount = %d, want 2", st.CrashCount)
	}
	if st.LastCrashReason != sb.lastCrashReason {
		t.Errorf("LastCrashReason = %q, want %q", st.LastCrashReason, sb.lastCrashReason)
	}
	if want := until.Format(time.RFC3339); st.RestartSuppressedUntil != want {
		t.Errorf("RestartSuppressedUntil = %q, want %q", st.RestartSuppressedUntil, want)
	}
}

// GetStatus без падений: поля-нули (omitempty на проводе).
func TestGetStatus_NoCrashesNoFields(t *testing.T) {
	settingsStore := newTestSettingsStore(t, storage.SingboxRouterSettings{
		RoutingMode:   "tproxy",
		PolicyName:    "Policy0",
		WANAutoDetect: true,
	})
	svc := newTestService(t, Deps{
		Settings: settingsStore,
		Singbox:  newTestSingbox(t),
		IPTables: errProbeIPTables(),
		Policies: &fakeAccessPolicyProvider{},
	})

	st, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if st.CrashCount != 0 || st.LastCrashReason != "" || st.RestartSuppressedUntil != "" {
		t.Errorf("want zero crash fields, got count=%d reason=%q until=%q",
			st.CrashCount, st.LastCrashReason, st.RestartSuppressedUntil)
	}
}

// chainsOnlyDump: цепочки AWGM живы, PREROUTING-джампов нет — состояние
// «NDMS перетёр PREROUTING», которое jump-heal обычно долечивает.
func chainsOnlyDump() string {
	return "-P PREROUTING ACCEPT\n" +
		"-N " + ChainName + "\n" +
		"-N " + RedirectChain + "\n"
}

// Fail-closed: движок мёртв, рестарт подавлен И PREROUTING-джампы снесены
// (chainsOnlyDump) → НЕ восстанавливаем перехват в мёртвый порт, а ставим
// blackhole-DROP policy-трафика, чтобы он не утёк в WAN. Ровно один restore —
// blackhole-блоб, не реальный перехват; флаг blackholeActive выставлен.
func TestReconcileInstalled_DeadEngineInstallsBlackhole(t *testing.T) {
	sb := newTestSingbox(t) // IsRunning=false весь тест
	svc := newReconcileInstalledService(t, sb)
	var restores []string
	ipt := newStubIPTables(func(_ context.Context, in string) error { restores = append(restores, in); return nil })
	ipt.runIPTablesOut = func(_ context.Context, _ ...string) (string, error) { return chainsOnlyDump(), nil }
	svc.deps.IPTables = ipt

	if err := svc.reconcileInstalled(context.Background(), reconcileInstalledSettings); err != nil {
		t.Fatalf("reconcileInstalled: %v", err)
	}
	if len(restores) != 1 {
		t.Fatalf("restore calls = %d, want 1 (blackhole only, no real interception)", len(restores))
	}
	if !strings.Contains(restores[0], "-A "+BlackholeChain+" -j DROP") {
		t.Errorf("restored blob is not the fail-closed blackhole:\n%s", restores[0])
	}
	if strings.Contains(restores[0], ChainName) {
		t.Errorf("must NOT restore real interception (%s) into a dead port:\n%s", ChainName, restores[0])
	}
	if !svc.blackholeActive {
		t.Error("blackholeActive must be set after installing the fail-closed blackhole")
	}
}

// safety-3: движок ЖИВ, но inbound-сокеты НЕ привязаны (up-but-unbound, порт
// занят / отклонённый hot-reload). reconcile трактует это как «не готов»:
// НЕ ставит реальный перехват (REDIRECT/TPROXY в непривязанный сокет
// заблэкхолил бы весь policy-трафик, вкл. DNS:53), а при снесённых джампах
// включает fail-closed blackhole.
func TestReconcileInstalled_LiveButUnboundInstallsBlackhole(t *testing.T) {
	sb := newTestSingbox(t)
	sb.isRunningFn = func() (bool, int) { return true, 1234 } // процесс жив
	var restores []string
	ipt := newStubIPTables(func(_ context.Context, in string) error { restores = append(restores, in); return nil })
	ipt.runIPTablesOut = func(_ context.Context, _ ...string) (string, error) { return chainsOnlyDump(), nil } // джампы снесены
	svc := newReconcileInstalledService(t, sb)
	svc.deps.IPTables = ipt
	// ПОСЛЕ харнесса (тот стабит probe=true) переопределяем: сокеты не привязаны.
	stubListeningProbe(t, func() bool { return false })

	if err := svc.reconcileInstalled(context.Background(), reconcileInstalledSettings); err != nil {
		t.Fatalf("reconcileInstalled: %v", err)
	}
	if len(restores) != 1 {
		t.Fatalf("restore calls = %d, want 1 (blackhole only, no real interception into an unbound socket)", len(restores))
	}
	if !strings.Contains(restores[0], "-A "+BlackholeChain+" -j DROP") {
		t.Errorf("restored blob is not the fail-closed blackhole:\n%s", restores[0])
	}
	if strings.Contains(restores[0], ChainName) {
		t.Errorf("must NOT install real interception (%s) while sockets are unbound:\n%s", ChainName, restores[0])
	}
	if !svc.blackholeActive {
		t.Error("blackholeActive must be set for an up-but-unbound engine")
	}
}

// Regression (ревью lifecycle): probe-ОШИБКА при мёртвом движке НЕ должна
// снимать blackhole. Иначе транзиентная -S ошибка во время NDMS-reload (ровно
// когда blackhole и нужен) снесла бы DROP при живой утечке. blackhole сохраняем.
func TestReconcileInstalled_ProbeErrorPreservesBlackhole(t *testing.T) {
	sb := newTestSingbox(t) // dead
	svc := newReconcileInstalledService(t, sb)
	svc.blackholeActive = true // прошлый тик поставил blackhole
	removed := false
	ipt := newStubIPTables(func(_ context.Context, _ string) error { return nil })
	ipt.runIPTablesOut = func(_ context.Context, _ ...string) (string, error) {
		return "", errors.New("iptables -S failed (NDMS reload)")
	}
	ipt.cleanupBlackhole = func() { removed = true }
	svc.deps.IPTables = ipt

	if err := svc.reconcileInstalled(context.Background(), reconcileInstalledSettings); err != nil {
		t.Fatalf("reconcileInstalled: %v", err)
	}
	if removed || !svc.blackholeActive {
		t.Errorf("probe error + dead engine must PRESERVE blackhole: removed=%v active=%v", removed, svc.blackholeActive)
	}
}

// Движок вернулся (jumps present, IsRunning=true) → ранее поставленный
// fail-closed blackhole снимается: cleanupBlackhole вызван, флаг сброшен.
func TestReconcileInstalled_EngineRecoveryRemovesBlackhole(t *testing.T) {
	sb := newTestSingbox(t)
	sb.isRunningFn = func() (bool, int) { return true, 4242 } // alive
	svc := newReconcileInstalledService(t, sb)
	svc.blackholeActive = true // как будто прошлый тик поставил blackhole
	removed := false
	ipt := newStubIPTables(func(_ context.Context, _ string) error { return nil })
	ipt.cleanupBlackhole = func() { removed = true }
	svc.deps.IPTables = ipt

	if err := svc.reconcileInstalled(context.Background(), reconcileInstalledSettings); err != nil {
		t.Fatalf("reconcileInstalled: %v", err)
	}
	if !removed || svc.blackholeActive {
		t.Errorf("engine recovery must drop the blackhole: cleanup=%v active=%v", removed, svc.blackholeActive)
	}
}

// FIX-B контроль: живой движок → jump-heal работает как раньше
// (отсутствующие PREROUTING-джампы восстанавливаются re-Install'ом).
func TestReconcileInstalled_AliveEngineStillHealsJumps(t *testing.T) {
	sb := newTestSingbox(t)
	sb.isRunningFn = func() (bool, int) { return true, 1234 }
	svc := newReconcileInstalledService(t, sb)
	installs := 0
	ipt := newStubIPTables(func(_ context.Context, _ string) error { installs++; return nil })
	ipt.runIPTablesOut = func(_ context.Context, _ ...string) (string, error) { return chainsOnlyDump(), nil }
	svc.deps.IPTables = ipt

	if err := svc.reconcileInstalled(context.Background(), reconcileInstalledSettings); err != nil {
		t.Fatalf("reconcileInstalled: %v", err)
	}
	if installs != 1 {
		t.Errorf("Install calls = %d, want 1 (jump heal must proceed with a live engine)", installs)
	}
}

// FIX-B: мёртвый движок при живом перехвате виден пользователю — GetStatus
// добавляет issue «Движок остановлен, но перехват трафика активен…» с
// временем окончания паузы и счётчиком падений.
func TestGetStatus_DeadEngineWithInterceptionIssue(t *testing.T) {
	stubListeningProbe(t, func() bool { return false })
	sb := newTestSingbox(t) // IsRunning=false
	sb.crashCount = 3
	sb.restartSuppressedUntil = time.Now().Add(10 * time.Minute)

	settingsStore := newTestSettingsStore(t, storage.SingboxRouterSettings{
		Enabled:       true,
		RoutingMode:   "tproxy",
		PolicyName:    "Policy0",
		WANAutoDetect: true,
	})
	ipt := newStubIPTables(func(_ context.Context, _ string) error { return nil }) // jumps present
	svc := newTestService(t, Deps{
		Settings: settingsStore,
		Singbox:  sb,
		IPTables: ipt,
		Policies: &fakeAccessPolicyProvider{},
	})

	st, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	var issue *Issue
	for i := range st.Issues {
		if st.Issues[i].Kind == "engine-dead-interception" {
			issue = &st.Issues[i]
			break
		}
	}
	if issue == nil {
		t.Fatalf("want engine-dead-interception issue, got %+v", st.Issues)
	}
	if issue.Severity != "error" {
		t.Errorf("severity = %q, want error", issue.Severity)
	}
	if !strings.Contains(issue.Message, "Движок остановлен, но перехват трафика активен") {
		t.Errorf("message = %q, want dead-engine wording", issue.Message)
	}
	if !strings.Contains(issue.Message, "приостановлен до") || !strings.Contains(issue.Message, "падений за 10 мин: 3") {
		t.Errorf("message = %q, want suppression time and crash count", issue.Message)
	}

	// Контроль: живой движок → issue нет.
	sb.isRunningFn = func() (bool, int) { return true, 4242 }
	st, err = svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus (alive): %v", err)
	}
	for _, i := range st.Issues {
		if i.Kind == "engine-dead-interception" {
			t.Fatalf("alive engine must not carry the issue, got %+v", st.Issues)
		}
	}
}
