package selective

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hoaxisr/awg-manager/internal/logging"
)

// resetIPSetHealthForTest полностью очищает кэш health-check'а (включая
// lastErr, который прод-resetIPSetCache сознательно сохраняет) — тесты
// не должны видеть вердикты и дедуп-состояние друг друга.
func resetIPSetHealthForTest() {
	ipsetHealth.mu.Lock()
	defer ipsetHealth.mu.Unlock()
	ipsetHealth.path = ""
	ipsetHealth.checked = time.Time{}
	ipsetHealth.lastErr = map[string]string{}
}

// writeStubIPSet кладёт исполняемый скрипт-заглушку ipset во временную
// директорию и возвращает путь.
func writeStubIPSet(t *testing.T, script string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "ipset")
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// captureAppLog собирает вызовы AppLog для проверки журналирования вердиктов.
type captureAppLog struct {
	mu      sync.Mutex
	entries []string // "LEVEL action target message"
}

func (c *captureAppLog) AppLog(level logging.Level, group, subgroup, action, target, message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, fmt.Sprintf("%s %s %s %s", level, action, target, message))
}

func (c *captureAppLog) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.entries...)
}

// withHealthTestEnv подменяет candidate paths, пробу и журнал health-check'а,
// восстанавливая всё по завершении теста.
func withHealthTestEnv(t *testing.T, paths []string, probe func(path string) error) *captureAppLog {
	t.Helper()
	origPaths := ipsetBinaryPaths
	origProbe := probeIPSetBinary
	origLog := healthLog
	capture := &captureAppLog{}
	ipsetBinaryPaths = paths
	if probe != nil {
		probeIPSetBinary = probe
	}
	healthLog = logging.NewScopedLogger(capture, logging.GroupRouting, logging.SubSelective)
	resetIPSetHealthForTest()
	t.Cleanup(func() {
		ipsetBinaryPaths = origPaths
		probeIPSetBinary = origProbe
		healthLog = origLog
		resetIPSetHealthForTest()
	})
	return capture
}

// Битый первый кандидат (существует, но не исполняется) пропускается с Warn,
// выбирается следующий рабочий; повторный вызов отдаёт кэш без новых проб.
func TestIPSetBinary_SkipsBrokenCandidate(t *testing.T) {
	broken := writeStubIPSet(t, "#!/bin/sh\nexit 1\n")
	good := writeStubIPSet(t, "#!/bin/sh\nexit 0\n")

	var probes []string
	var probesMu sync.Mutex
	capture := withHealthTestEnv(t, []string{broken, good}, func(path string) error {
		probesMu.Lock()
		probes = append(probes, path)
		probesMu.Unlock()
		if path == broken {
			return errors.New("error while loading shared libraries: libc.so")
		}
		return nil
	})

	if got := IPSetBinary(); got != good {
		t.Fatalf("expected %q, got %q", good, got)
	}
	if got := IPSetBinary(); got != good { // из кэша
		t.Fatalf("cached call: expected %q, got %q", good, got)
	}
	probesMu.Lock()
	n := len(probes)
	probesMu.Unlock()
	if n != 2 { // broken + good, второй вызов IPSetBinary проб не делает
		t.Errorf("expected 2 probes, got %d: %v", n, probes)
	}

	logs := capture.all()
	if len(logs) != 1 {
		t.Fatalf("expected exactly 1 log entry, got %d: %v", len(logs), logs)
	}
	if !strings.Contains(logs[0], "warn") && !strings.Contains(logs[0], "WARN") {
		t.Errorf("expected warn-level entry, got %q", logs[0])
	}
	if !strings.Contains(logs[0], broken) || !strings.Contains(logs[0], "libc.so") {
		t.Errorf("warn must name the broken path and error, got %q", logs[0])
	}
	if !strings.Contains(logs[0], "force-reinstall") {
		t.Errorf("warn must carry the remediation hint, got %q", logs[0])
	}
}

// Все кандидаты битые: результат "" кэшируется (без проб на каждый вызов),
// повторный идентичный сбой не дублирует Warn, а восстановление после
// resetIPSetCache журналируется как Info-переход.
func TestIPSetBinary_BrokenThenRecovered(t *testing.T) {
	bin := writeStubIPSet(t, "#!/bin/sh\nexit 0\n")

	var healthy bool
	var healthyMu sync.Mutex
	capture := withHealthTestEnv(t, []string{bin}, func(string) error {
		healthyMu.Lock()
		defer healthyMu.Unlock()
		if healthy {
			return nil
		}
		return errors.New("exit status 127")
	})

	if got := IPSetBinary(); got != "" {
		t.Fatalf("expected empty for broken binary, got %q", got)
	}
	if got := IPSetBinary(); got != "" { // отрицательный вердикт тоже из кэша
		t.Fatalf("cached call: expected empty, got %q", got)
	}
	if logs := capture.all(); len(logs) != 1 {
		t.Fatalf("expected 1 warn, got %d: %v", len(logs), logs)
	}

	// Тот же сбой после сброса кэша — Warn не дублируется.
	resetIPSetCache()
	if got := IPSetBinary(); got != "" {
		t.Fatalf("still broken: expected empty, got %q", got)
	}
	if logs := capture.all(); len(logs) != 1 {
		t.Fatalf("identical failure must not re-warn, got %d: %v", len(logs), logs)
	}

	// «Переустановка пакета»: бинарь ожил, InstallIPSet сбрасывает кэш.
	healthyMu.Lock()
	healthy = true
	healthyMu.Unlock()
	resetIPSetCache()
	if got := IPSetBinary(); got != bin {
		t.Fatalf("after recovery: expected %q, got %q", bin, got)
	}
	logs := capture.all()
	if len(logs) != 2 {
		t.Fatalf("expected warn+info, got %d: %v", len(logs), logs)
	}
	if !strings.Contains(logs[1], "runnable again") {
		t.Errorf("expected recovery info entry, got %q", logs[1])
	}
}

// Дефолтная проба (реальный запуск `<bin> version`) отличает исполняемый
// скрипт от падающего — покрывает sysexec-путь без подмены probeIPSetBinary.
func TestProbeIPSetBinary_RealExecution(t *testing.T) {
	good := writeStubIPSet(t, "#!/bin/sh\nexit 0\n")
	if err := probeIPSetBinary(good); err != nil {
		t.Errorf("healthy stub must pass probe, got %v", err)
	}

	broken := writeStubIPSet(t, "#!/bin/sh\necho 'error while loading shared libraries: libc.so' >&2\nexit 127\n")
	err := probeIPSetBinary(broken)
	if err == nil {
		t.Fatal("broken stub must fail probe")
	}
	if !strings.Contains(err.Error(), "libc.so") {
		t.Errorf("probe error must surface stderr, got %v", err)
	}
}
