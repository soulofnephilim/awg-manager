package singbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readMigrFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMigrateRuleSetURLsToFork(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{filepath.Join(dir, "disabled"), filepath.Join(dir, "pending")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	vern := `{"route":{"rule_set":[{"tag":"x","type":"remote","url":"https://github.com/vernette/rulesets/raw/master/srs/discord-full.srs","unknown_field":"keep-me"}]}}`
	clean := `{"route":{"rule_set":[]}}`

	// vernette живёт не только в 20-router.json: 21-fakeip.json тоже держит
	// remote rule-set URL; миграция обязана подмести ВСЕ slot-файлы в active/
	// disabled/pending.
	withVernette := []string{
		filepath.Join(dir, "20-router.json"),
		filepath.Join(dir, "21-fakeip.json"),
		filepath.Join(dir, "disabled", "20-router.json"),
		filepath.Join(dir, "pending", "21-fakeip.json"),
	}
	for _, p := range withVernette {
		if err := os.WriteFile(p, []byte(vern), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	untouched := filepath.Join(dir, "30-deviceproxy.json")
	if err := os.WriteFile(untouched, []byte(clean), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := MigrateRuleSetURLsToFork(dir); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	for _, p := range withVernette {
		s := readMigrFile(t, p)
		if strings.Contains(s, "vernette") {
			t.Errorf("%s: vernette осталось", p)
		}
		if !strings.Contains(s, "https://repo.hoaxisr.ru/rulesets/srs/discord-full.srs") {
			t.Errorf("%s: нет URL зеркала", p)
		}
		if !strings.Contains(s, "keep-me") {
			t.Errorf("%s: неизвестное поле потеряно", p)
		}
	}
	if readMigrFile(t, untouched) != clean {
		t.Errorf("файл без vernette изменён")
	}

	// Идемпотентность.
	before := readMigrFile(t, withVernette[0])
	if err := MigrateRuleSetURLsToFork(dir); err != nil {
		t.Fatalf("migrate (2nd): %v", err)
	}
	if readMigrFile(t, withVernette[0]) != before {
		t.Errorf("2-й прогон изменил файл")
	}
}

func TestMigrateRuleSetURLsToFork_NoDir(t *testing.T) {
	if err := MigrateRuleSetURLsToFork(filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Fatalf("ожидался no-op, got %v", err)
	}
}
