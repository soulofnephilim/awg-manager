package selective

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCDNQueriesFromConfigDir(t *testing.T) {
	dir := t.TempDir()
	w, err := newSnapshotWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = w.WriteRecord(DomainMatcherRecord{Matcher: "cdn.example.com", Kind: "suffix", CDN: true})
	_ = w.WriteRecord(DomainMatcherRecord{Matcher: "plain.example.com", Kind: "suffix"})
	_ = w.WriteRecord(DomainMatcherRecord{Matcher: "broken.example.com", Kind: "suffix", CDN: true, Error: "no A"})
	if err := w.CloseAndCommit(SnapshotSummary{RebuiltAt: "x", EntryCount: 1}); err != nil {
		t.Fatal(err)
	}
	got, err := CDNQueriesFromConfigDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Matcher != "cdn.example.com" {
		t.Fatalf("expected only CDN matcher without error, got %v", got)
	}
}

func TestMergeRoutesLocked(t *testing.T) {
	b := &Builder{routes: map[string][]string{
		"vpn": {"1.1.1.1/32"},
	}}

	// New IP for an existing outbound + a brand-new outbound → both merged.
	n := b.mergeRoutesLocked(map[string][]string{
		"vpn":   {"1.1.1.1/32", "2.2.2.2/32"},
		"proxy": {"3.3.3.3/32"},
	})
	if n != 2 {
		t.Fatalf("merged = %d, want 2", n)
	}
	if got := len(b.routes["vpn"]); got != 2 {
		t.Fatalf("vpn routes = %d, want 2", got)
	}
	if got := len(b.routes["proxy"]); got != 1 {
		t.Fatalf("proxy routes = %d, want 1", got)
	}

	// Re-merging the same set is a no-op — the caller must not reload.
	if n := b.mergeRoutesLocked(map[string][]string{
		"vpn":   {"2.2.2.2/32"},
		"proxy": {"3.3.3.3/32"},
	}); n != 0 {
		t.Fatalf("re-merge = %d, want 0", n)
	}

	// Nil map on a builder with no routes stays nil-safe.
	empty := &Builder{}
	if n := empty.mergeRoutesLocked(nil); n != 0 {
		t.Fatalf("nil merge = %d, want 0", n)
	}
}

func TestReadSnapshotMatchersPagination(t *testing.T) {
	dir := t.TempDir()
	w, err := newSnapshotWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		_ = w.WriteRecord(DomainMatcherRecord{Matcher: "host.example", Kind: "domain"})
	}
	if err := w.CloseAndCommit(SnapshotSummary{EntryCount: 5}); err != nil {
		t.Fatal(err)
	}
	page, total, err := ReadSnapshotMatchers(dir, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 || len(page) != 2 {
		t.Fatalf("total=%d page=%d", total, len(page))
	}
	if _, err := os.Stat(filepath.Join(dir, snapshotMetaFile)); err != nil {
		t.Fatalf("meta file missing: %v", err)
	}
}
