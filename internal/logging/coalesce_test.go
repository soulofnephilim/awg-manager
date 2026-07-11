package logging

import (
	"testing"
	"time"
)

func testEntry(msg string, ts time.Time) LogEntry {
	return LogEntry{
		Timestamp: ts,
		Level:     "warn",
		Group:     GroupTunnel,
		Subgroup:  SubConnectivity,
		Action:    "http-check",
		Target:    "awg10",
		Message:   msg,
	}
}

func TestCoalesce_FoldsIdenticalRepeat(t *testing.T) {
	lb := NewLogBuffer(BucketApp)
	defer lb.Stop()

	base := time.Now()
	lb.Add(testEntry("fail", base))

	updated, ok := lb.Coalesce(testEntry("fail", base.Add(30*time.Second)), 5*time.Minute)
	if !ok {
		t.Fatal("expected repeat to coalesce")
	}
	if updated.Repeats != 1 {
		t.Fatalf("Repeats = %d, want 1", updated.Repeats)
	}
	if updated.LastSeen == nil || !updated.LastSeen.Equal(base.Add(30*time.Second)) {
		t.Fatalf("LastSeen = %v, want %v", updated.LastSeen, base.Add(30*time.Second))
	}
	if !updated.Timestamp.Equal(base) {
		t.Fatalf("Timestamp must stay first-seen: %v != %v", updated.Timestamp, base)
	}
	if lb.Len() != 1 {
		t.Fatalf("Len = %d, want 1", lb.Len())
	}
}

func TestCoalesce_LastSeenExtendsWindow(t *testing.T) {
	lb := NewLogBuffer(BucketApp)
	defer lb.Stop()

	base := time.Now()
	lb.Add(testEntry("fail", base))

	// Повторы каждые 4 минуты при окне 5 минут: каждый следующий попадает
	// в окно от LastSeen предыдущего, хотя от Timestamp уже далеко.
	for i := 1; i <= 3; i++ {
		ts := base.Add(time.Duration(i) * 4 * time.Minute)
		updated, ok := lb.Coalesce(testEntry("fail", ts), 5*time.Minute)
		if !ok {
			t.Fatalf("repeat %d must coalesce (window extends via LastSeen)", i)
		}
		if updated.Repeats != i {
			t.Fatalf("repeat %d: Repeats = %d", i, updated.Repeats)
		}
	}
	if lb.Len() != 1 {
		t.Fatalf("Len = %d, want 1", lb.Len())
	}
}

func TestCoalesce_ExpiredWindowStartsFresh(t *testing.T) {
	lb := NewLogBuffer(BucketApp)
	defer lb.Stop()

	base := time.Now()
	lb.Add(testEntry("fail", base))

	if _, ok := lb.Coalesce(testEntry("fail", base.Add(6*time.Minute)), 5*time.Minute); ok {
		t.Fatal("repeat outside window must NOT coalesce")
	}
}

func TestCoalesce_DifferentFieldsDoNotFold(t *testing.T) {
	lb := NewLogBuffer(BucketApp)
	defer lb.Stop()

	base := time.Now()
	lb.Add(testEntry("fail", base))

	other := testEntry("fail", base.Add(time.Second))
	other.Target = "awg11"
	if _, ok := lb.Coalesce(other, 5*time.Minute); ok {
		t.Fatal("different target must not coalesce")
	}
	other = testEntry("different message", base.Add(time.Second))
	if _, ok := lb.Coalesce(other, 5*time.Minute); ok {
		t.Fatal("different message must not coalesce")
	}
	other = testEntry("fail", base.Add(time.Second))
	other.Level = "error"
	if _, ok := lb.Coalesce(other, 5*time.Minute); ok {
		t.Fatal("different level must not coalesce")
	}
}

func TestCoalesce_InterleavedEntriesStillFold(t *testing.T) {
	lb := NewLogBuffer(BucketApp)
	defer lb.Stop()

	base := time.Now()
	lb.Add(testEntry("fail A", base))
	lb.Add(testEntry("fail B", base.Add(time.Second)))

	// Повтор A находит свою запись сквозь более новую B.
	updated, ok := lb.Coalesce(testEntry("fail A", base.Add(2*time.Second)), 5*time.Minute)
	if !ok || updated.Message != "fail A" || updated.Repeats != 1 {
		t.Fatalf("interleaved repeat must coalesce into its own entry: ok=%v entry=%+v", ok, updated)
	}
	if lb.Len() != 2 {
		t.Fatalf("Len = %d, want 2", lb.Len())
	}
}

func TestServiceAppLog_CoalescesRepeats(t *testing.T) {
	s := NewService(&mockSettings{enabled: true, maxAge: 2, logLevel: "info", appMaxEntries: 100, sbMaxEntries: 100})
	defer s.Stop()

	for i := 0; i < 5; i++ {
		s.AppLog(LevelWarn, GroupTunnel, SubConnectivity, "http-check", "awg10", "HTTP check failed: timeout")
	}
	entries, total := s.GetLogs(BucketApp, "", "", "", time.Time{}, 10, 0)
	if total != 1 || len(entries) != 1 {
		t.Fatalf("total = %d, len = %d, want 1", total, len(entries))
	}
	if entries[0].Repeats != 4 {
		t.Fatalf("Repeats = %d, want 4", entries[0].Repeats)
	}
	if entries[0].LastSeen == nil {
		t.Fatal("LastSeen must be set on coalesced entry")
	}

	// Другая запись не сворачивается и не мешает следующему повтору свернуться.
	s.AppLog(LevelInfo, GroupTunnel, SubLifecycle, "start", "awg11", "Tunnel started")
	s.AppLog(LevelWarn, GroupTunnel, SubConnectivity, "http-check", "awg10", "HTTP check failed: timeout")
	_, total = s.GetLogs(BucketApp, "", "", "", time.Time{}, 10, 0)
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
}
