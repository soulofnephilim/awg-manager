package router

// Тесты синхронного моста «роутер → зависимые продюсеры» (issue #465):
// Enable/Disable обязаны дернуть OnRoutingSlotsChanged ПОСЛЕ перепарковки
// 20-router.json и ДО reload, чтобы device-proxy успел перегенерировать
// слот 30 (деградация/восстановление ссылок на композиты) до того, как
// prune оркестратора вырежет висячие ссылки молча.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hoaxisr/awg-manager/internal/singbox/orchestrator"
	"github.com/hoaxisr/awg-manager/internal/storage"
)

// ensureDisabledDir: newQoSSlotTestService не вызывает Bootstrap, поэтому
// каталога disabled/ (куда SetEnabled парклит слоты) ещё нет.
func ensureDisabledDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "disabled"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func routerSlotEnabled(o *orchestrator.Orchestrator) bool {
	for _, st := range o.Snapshot() {
		if st.Slot == orchestrator.SlotRouter {
			return st.Enabled
		}
	}
	return false
}

func TestDisable_Tproxy_NotifiesRoutingSlotsChanged(t *testing.T) {
	svc, dir := newQoSSlotTestService(t, "vpn")
	ensureDisabledDir(t, dir)
	orch := svc.deps.Orch

	var notified int
	var slotEnabledAtNotify bool
	svc.deps.OnRoutingSlotsChanged = func() {
		notified++
		slotEnabledAtNotify = routerSlotEnabled(orch)
	}
	svc.deps.Settings = newTestSettingsStore(t, storage.SingboxRouterSettings{
		RoutingMode:   "tproxy",
		DeviceMode:    "all",
		WANAutoDetect: true,
		Enabled:       true,
	})
	svc.deps.Singbox = &fakeSingbox{dir: dir, isRunningFn: func() (bool, int) { return true, 1234 }}
	svc.deps.IPTables = newStubIPTables(func(_ context.Context, _ string) error { return nil })

	if err := svc.Disable(context.Background()); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if notified != 1 {
		t.Fatalf("OnRoutingSlotsChanged calls = %d, want 1", notified)
	}
	// Хук должен видеть УЖЕ припаркованный слот 20 — иначе device-proxy
	// перегенерирует слот 30 по устаревшей видимости композитов.
	if slotEnabledAtNotify {
		t.Error("hook fired BEFORE the router slot was parked")
	}
}

func TestEnable_Tproxy_NotifiesRoutingSlotsChanged(t *testing.T) {
	svc, dir := newQoSSlotTestService(t, "vpn")
	ensureDisabledDir(t, dir)
	orch := svc.deps.Orch
	// Стартуем из выключенного состояния (файл в disabled/).
	if err := orch.SetEnabledSilent(orchestrator.SlotRouter, false); err != nil {
		t.Fatalf("park router slot: %v", err)
	}

	var notified int
	var slotEnabledAtNotify bool
	svc.deps.OnRoutingSlotsChanged = func() {
		notified++
		slotEnabledAtNotify = routerSlotEnabled(orch)
	}
	svc.deps.Settings = newTestSettingsStore(t, storage.SingboxRouterSettings{
		RoutingMode:   "tproxy",
		DeviceMode:    "all",
		WANAutoDetect: true,
	})
	svc.deps.Singbox = &fakeSingbox{dir: dir, isRunningFn: func() (bool, int) { return true, 1234 }}
	stubListeningProbe(t, func() bool { return true })
	svc.deps.Policies = &fakeAccessPolicyProvider{}
	svc.deps.IPTables = newStubIPTables(func(_ context.Context, _ string) error { return nil })
	svc.deps.WANIPCollector = &fakeWANIPCollector{}
	svc.deps.NetfilterPreflight = func(context.Context) error { return nil }
	svc.deps.XtDscpProbe = func(context.Context) bool { return true }

	if err := svc.Enable(context.Background()); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if notified == 0 {
		t.Fatal("OnRoutingSlotsChanged not invoked on Enable")
	}
	if !slotEnabledAtNotify {
		t.Error("hook fired BEFORE the router slot became active")
	}
}
