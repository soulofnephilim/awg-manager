// Package selective implements the selective-bypass feature for the sing-box
// TProxy router: only traffic whose destination IP is listed in the
// AWGM-SELECTIVE ipset reaches sing-box; all other traffic bypasses it
// entirely (RETURN → WAN).
package selective

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hoaxisr/awg-manager/internal/logging"
	sysexec "github.com/hoaxisr/awg-manager/internal/sys/exec"
	"github.com/hoaxisr/awg-manager/internal/sys/osdetect"
)

// ipsetBinaryPaths lists candidate absolute paths for the ipset binary,
// searched in order. Entware installs to /opt/sbin; system may have
// /usr/sbin or /sbin.
var ipsetBinaryPaths = []string{
	"/opt/sbin/ipset",
	"/usr/sbin/ipset",
	"/sbin/ipset",
}

// healthLog — журнал приложения (routing/selective) для вердиктов health-check
// бинаря ipset. nil-safe: до/без подключения через SetHealthLogger вердикты
// просто не журналируются.
var healthLog *logging.ScopedLogger

// SetHealthLogger подключает журнал для health-check ipset. Вызывается один
// раз из wiring; пишется под тем же мьютексом, под которым IPSetBinary
// журналирует вердикты, — безопасно и при уже работающих фоновых вызовах.
func SetHealthLogger(l *logging.ScopedLogger) {
	ipsetHealth.mu.Lock()
	defer ipsetHealth.mu.Unlock()
	healthLog = l
}

// ipsetProbeTimeout ограничивает пробу `ipset version`: сломанный бинарь
// (exit 127) падает мгновенно, здоровый отвечает миллисекунды — таймаут
// нужен только против зависшего носителя /opt.
const ipsetProbeTimeout = 5 * time.Second

const (
	// ipsetHealthyTTL — как долго доверять найденному рабочему бинарю,
	// прежде чем перепроверить (opkg upgrade может сломать его на лету).
	ipsetHealthyTTL = 5 * time.Minute
	// ipsetBrokenTTL — как долго кэшировать вердикт «рабочего бинаря нет»:
	// короче, чтобы ручная починка (переустановка пакета по SSH) подхватилась
	// без рестарта демона.
	ipsetBrokenTTL = 30 * time.Second
)

// probeIPSetBinary запускает `<path> version` и возвращает ошибку, если
// бинарь не исполняется (битая установка Entware: «error while loading
// shared libraries: libc.so» даёт exit 127 при живом файле — голый os.Stat
// такое пропускает, а все реальные команды потом молча падают).
// Подменяется в тестах.
var probeIPSetBinary = func(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), ipsetProbeTimeout)
	defer cancel()
	res, err := sysexec.RunWithOptions(ctx, path, []string{"version"}, sysexec.Options{Timeout: ipsetProbeTimeout})
	if err != nil {
		return sysexec.FormatError(res, err)
	}
	return nil
}

// ipsetHealth кэширует результат поиска рабочего бинаря ipset. Пробы идут
// под мьютексом — конкурентные вызовы (status-поллинг + пересборка) ждут
// один общий вердикт, а не форкают ipset наперегонки.
var ipsetHealth = struct {
	mu      sync.Mutex
	path    string    // проверенный рабочий путь ("" — нет)
	checked time.Time // момент вердикта (zero — кэш пуст)
	// lastErr хранит последний зажурналенный сбой по каждому кандидату:
	// повторный идентичный вердикт не спамит Warn, смена ошибки и
	// восстановление — журналируются.
	lastErr map[string]string
}{lastErr: map[string]string{}}

// IPSetBinary returns the path to a WORKING ipset binary, or "" when no
// candidate both exists and executes. Существующий, но неисполнимый бинарь
// (битый Entware-пакет) пропускается с Warn в журнал — иначе он затеняет
// рабочий системный и валит каждую команду exit-кодом 127.
func IPSetBinary() string {
	now := time.Now()
	ipsetHealth.mu.Lock()
	defer ipsetHealth.mu.Unlock()

	ttl := ipsetBrokenTTL
	if ipsetHealth.path != "" {
		ttl = ipsetHealthyTTL
	}
	if !ipsetHealth.checked.IsZero() && now.Sub(ipsetHealth.checked) < ttl {
		return ipsetHealth.path
	}

	chosen := ""
	for _, p := range ipsetBinaryPaths {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if err := probeIPSetBinary(p); err != nil {
			msg := err.Error()
			if ipsetHealth.lastErr[p] != msg {
				ipsetHealth.lastErr[p] = msg
				healthLog.Warn("ipset-health", p,
					fmt.Sprintf("ipset binary is present but not runnable (%s) — candidate skipped; reinstall it: opkg install --force-reinstall ipset", msg))
			}
			continue
		}
		if ipsetHealth.lastErr[p] != "" {
			delete(ipsetHealth.lastErr, p)
			healthLog.Info("ipset-health", p, "ipset binary is runnable again")
		}
		chosen = p
		break
	}
	ipsetHealth.path = chosen
	ipsetHealth.checked = now
	return chosen
}

// IsIPSetAvailable reports whether a working ipset binary is present on the
// router (existence AND executability — see IPSetBinary).
func IsIPSetAvailable() bool {
	return IPSetBinary() != ""
}

// xtSetModuleName is the kernel module name for iptables ipset matching.
const xtSetModuleName = "xt_set"

// IsXtSetAvailable reports whether the xt_set kernel module is currently
// loaded OR available as a .ko file that can be loaded.
// NOT cached — called at status-check time, result must reflect reality
// after module load attempts.
func IsXtSetAvailable() bool {
	if isModuleLoaded(xtSetModuleName) {
		return true
	}
	kernel := osdetect.KernelRelease()
	if kernel == "" {
		return false
	}
	path := filepath.Join("/lib/modules", kernel, xtSetModuleName+".ko")
	_, err := os.Stat(path)
	return err == nil
}

// isModuleLoaded checks /proc/modules for the given module name.
// Identical to the helper in iptables.go; duplicated to keep the
// selective package self-contained and avoid an internal import cycle.
func isModuleLoaded(name string) bool {
	data, err := os.ReadFile("/proc/modules")
	if err != nil {
		return false
	}
	prefix := name + " "
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// EnsureXtSetModule attempts to load xt_set.ko via insmod. Soft-fail by
// design: if the module is built into the kernel there is no .ko file but
// it still works; a hard error here would be a false positive. The caller
// (builder) logs the warning and continues — the real failure surfaces at
// iptables-restore COMMIT time with a concrete message.
func EnsureXtSetModule(ctx context.Context) error {
	if isModuleLoaded(xtSetModuleName) {
		return nil
	}
	kernel := osdetect.KernelRelease()
	if kernel == "" {
		return nil // soft-fail: can't determine kernel version
	}
	path := filepath.Join("/lib/modules", kernel, xtSetModuleName+".ko")
	if _, err := os.Stat(path); err != nil {
		return nil // soft-fail: .ko absent → assume built-in or unavailable
	}
	_, err := sysexec.Run(ctx, "insmod", path)
	return err
}

// InstallIPSet runs `opkg install ipset` and streams output lines to
// progressFn (called once per line, nil = silent). Returns the first
// non-zero exit error, or nil on success.
func InstallIPSet(ctx context.Context, progressFn func(line string)) error {
	opkg, err := findOpkg()
	if err != nil {
		return err
	}
	// We use RunWithOptions with a long timeout — opkg downloads packages
	// and can take 30-60s on slow WAN.
	res, err := sysexec.RunWithOptions(ctx, opkg, []string{"install", "ipset"},
		sysexec.Options{Timeout: 120e9}) // 120s
	if res != nil && progressFn != nil {
		combined := res.Stdout
		if res.Stderr != "" {
			combined += res.Stderr
		}
		for _, line := range strings.Split(combined, "\n") {
			if l := strings.TrimSpace(line); l != "" {
				progressFn(l)
			}
		}
	}
	if err != nil {
		return sysexec.FormatError(res, err)
	}
	// Invalidate the cached result so subsequent IsIPSetAvailable calls
	// reflect the newly installed binary.
	resetIPSetCache()

	// Best-effort: also install conntrack so a routing change takes effect
	// immediately (existing flows get evicted) instead of waiting for old
	// connections to expire. Failure here is non-fatal — the selective guard
	// works without it, just with delayed effect on established flows.
	if !IsConntrackAvailable() {
		_ = InstallConntrackTools(ctx, progressFn)
	}
	return nil
}

// InstallConntrackTools installs the conntrack userspace binary via opkg.
// Keenetic Entware ships it as package "conntrack"; some feeds use
// "conntrack-tools" instead — we try both.
func InstallConntrackTools(ctx context.Context, progressFn func(line string)) error {
	opkg, err := findOpkg()
	if err != nil {
		return err
	}
	for _, pkg := range []string{"conntrack", "conntrack-tools"} {
		if err := opkgInstall(ctx, opkg, pkg, progressFn); err == nil {
			return nil
		}
	}
	return fmt.Errorf("opkg install conntrack: package not found in feed")
}

func opkgInstall(ctx context.Context, opkg, pkg string, progressFn func(line string)) error {
	res, err := sysexec.RunWithOptions(ctx, opkg, []string{"install", pkg},
		sysexec.Options{Timeout: 120e9})
	if res != nil && progressFn != nil {
		combined := res.Stdout
		if res.Stderr != "" {
			combined += res.Stderr
		}
		for _, line := range strings.Split(combined, "\n") {
			if l := strings.TrimSpace(line); l != "" {
				progressFn(l)
			}
		}
	}
	if err != nil {
		return sysexec.FormatError(res, err)
	}
	return nil
}

// resetIPSetCache сбрасывает кэш health-check'а — вызывается после установки
// пакета, чтобы следующий IPSetBinary перепробовал кандидатов немедленно, а
// не по TTL. lastErr сознательно не чистится: восстановление после
// переустановки журналируется как переход («runnable again»).
func resetIPSetCache() {
	ipsetHealth.mu.Lock()
	defer ipsetHealth.mu.Unlock()
	ipsetHealth.path = ""
	ipsetHealth.checked = time.Time{}
}

// findOpkg returns the absolute path to opkg, or an error if not found.
func findOpkg() (string, error) {
	for _, p := range []string{"/opt/bin/opkg", "/usr/bin/opkg", "/bin/opkg"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", ErrOpkgNotFound
}
