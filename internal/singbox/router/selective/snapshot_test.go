package selective

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestNeedsPopulationForExistingSet(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name        string
		entryCount  int
		lastRebuild time.Time
		summary     *SnapshotSummary
		want        bool
	}{
		{"no rebuild marker", 0, time.Time{}, nil, true},
		{"populated set", 42, now, &SnapshotSummary{EntryCount: 42}, false},
		{"deliberately empty build", 0, now, &SnapshotSummary{EntryCount: 0}, false},
		{"empty set but snapshot expected entries", 0, now, &SnapshotSummary{EntryCount: 17}, true},
		{"empty set without snapshot", 0, now, nil, true},
		// A 0-entry rebuild with matchers configured is an outage artifact
		// (DNS resolve failures are not rebuild errors), not a deliberate
		// empty set — boot repopulation must fire.
		{"0-entry build with domain matchers", 0, now, &SnapshotSummary{EntryCount: 0, DomainMatcherCount: 12}, true},
		{"0-entry build with static cidrs", 0, now, &SnapshotSummary{EntryCount: 0, StaticCIDRCount: 3}, true},
		{"0-entry build with both configured", 0, now, &SnapshotSummary{EntryCount: 0, StaticCIDRCount: 3, DomainMatcherCount: 12}, true},
	}
	for _, tc := range cases {
		if got := needsPopulationForExistingSet(tc.entryCount, tc.lastRebuild, tc.summary); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestNeedsPopulationForExistingSet_EmptyIntentionalFromDisk(t *testing.T) {
	dir := t.TempDir()
	writeLastRebuild(dir, time.Now())
	writeSnapshotMeta(dir, SnapshotSummary{RebuiltAt: "2026-01-01T00:00:00Z", EntryCount: 0})
	if needsPopulationForExistingSet(0, readLastRebuild(dir), readSnapshotSummary(dir)) {
		t.Fatal("deliberately-empty set must not re-trigger a boot rebuild")
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
