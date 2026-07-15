// internal/singbox/migrate_rulesets_fork.go
package singbox

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hoaxisr/awg-manager/internal/storage"
)

// MigrateRuleSetURLsToFork rewrites snapshotted vernette rule-set URLs to the
// repo.hoaxisr.ru mirror in every persisted sing-box slot file, so sing-box
// re-fetches the mirror's .srs without the user re-applying presets
// (raw.githubusercontent.com заблокирован у части провайдеров — #534).
// Rule-set URLs live in more than one slot (20-router.json AND 21-fakeip.json
// at least), so we sweep ALL *.json across active, disabled/ and pending/
// rather than naming files.
//
// Byte-level replace (not a config round-trip): the prefix appears only inside
// string values, so substitution is safe and preserves any fields the structs
// do not model; files without the prefix are left byte-for-byte untouched.
// Idempotent — after the swap no vernette prefix remains.
func MigrateRuleSetURLsToFork(configDir string) error {
	for _, pat := range []string{
		filepath.Join(configDir, "*.json"),
		filepath.Join(configDir, "disabled", "*.json"),
		filepath.Join(configDir, "pending", "*.json"),
	} {
		matches, err := filepath.Glob(pat)
		if err != nil {
			return fmt.Errorf("glob %s: %w", pat, err)
		}
		for _, p := range matches {
			if err := rewriteRuleSetForkURLs(p); err != nil {
				return err
			}
		}
	}
	return nil
}

func rewriteRuleSetForkURLs(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	replaced := bytes.ReplaceAll(data,
		[]byte("github.com/vernette/rulesets/raw/master/"),
		[]byte("repo.hoaxisr.ru/rulesets/"))
	if bytes.Equal(replaced, data) {
		return nil
	}
	return storage.AtomicWrite(path, replaced)
}
