package configmerge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeJSON(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestMerge_TwoFilesConcatInbounds(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, dir, "00-base.json", `{"inbounds":[{"tag":"a","type":"http"}]}`)
	writeJSON(t, dir, "10-tunnels.json", `{"inbounds":[{"tag":"b","type":"socks"}]}`)

	out, err := MergeDir(dir)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !strings.Contains(out, `"tag": "a"`) || !strings.Contains(out, `"tag": "b"`) {
		t.Errorf("merged output missing one of the inbound tags:\n%s", out)
	}
}
