package selective

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotPath_NotJSONInConfigDir(t *testing.T) {
	dir := "/opt/etc/awg-manager/singbox/config.d"
	if filepath.Ext(snapshotPath(dir)) != "" {
		t.Fatalf("snapshot must not use .json extension, got %q", snapshotPath(dir))
	}
}

func TestRemoveLegacySnapshotJSON(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, legacySnapshotJSON)
	if err := os.WriteFile(legacy, []byte(`{"rebuiltAt":"x"}`), 0644); err != nil {
		t.Fatal(err)
	}
	RemoveLegacySnapshotJSON(dir)
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy snapshot json should be removed, stat err=%v", err)
	}
}

func TestNormalizeRebuildSnapshot_NilSlices(t *testing.T) {
	snap := NormalizeRebuildSnapshot(&RebuildSnapshot{
		DomainResults: []DomainResolveResult{{
			Matcher: "example.com",
			Kind:    "suffix",
		}},
	})
	if snap.StaticCIDRs == nil || snap.DomainResults[0].IPs == nil || snap.DomainResults[0].QueryHosts == nil {
		t.Fatalf("expected empty slices, got %+v", snap)
	}
}

func TestWriteSnapshot_RemovesLegacyJSON(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, legacySnapshotJSON)
	if err := os.WriteFile(legacy, []byte(`{"rebuiltAt":"old"}`), 0644); err != nil {
		t.Fatal(err)
	}
	writeSnapshot(dir, RebuildSnapshot{RebuiltAt: "2026-01-01T00:00:00Z", EntryCount: 1})
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("legacy file still present after writeSnapshot")
	}
	if _, err := os.Stat(snapshotMetaPath(dir)); err != nil {
		t.Fatalf("meta snapshot missing: %v", err)
	}
}
