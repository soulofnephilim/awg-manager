package updater

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/hoaxisr/awg-manager/internal/downloader"
	"github.com/hoaxisr/awg-manager/internal/storage"
)

func mustParseTime(t *testing.T, layout, value string) time.Time {
	t.Helper()
	tm, err := time.Parse(layout, value)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return tm
}

func TestAutoInstallDue_ClockNotYetValid(t *testing.T) {
	// 1970-clock guard: before NTP sync the router's clock reads ~1970,
	// which must never be treated as "the scheduled window arrived".
	now := time.Date(1970, 1, 1, 12, 0, 0, 0, time.UTC)
	if autoInstallDue(now, nil, 7, "05:00") {
		t.Fatal("expected false for year < 2024, got true")
	}
}

func TestAutoInstallDue_NilMarker_BeforeWindow(t *testing.T) {
	now := time.Date(2026, 7, 17, 4, 0, 0, 0, time.UTC)
	if autoInstallDue(now, nil, 7, "05:00") {
		t.Fatal("expected false before HH:MM with nil marker")
	}
}

func TestAutoInstallDue_NilMarker_AfterWindow(t *testing.T) {
	// No marker yet — due at the very next HH:MM window, no N-day wait.
	now := time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC)
	if !autoInstallDue(now, nil, 7, "05:00") {
		t.Fatal("expected true after HH:MM with nil marker")
	}
}

func TestAutoInstallDue_FreshMarker_NotDue(t *testing.T) {
	now := time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC)
	marker := &autoInstallMarker{LastAttemptAt: now.AddDate(0, 0, -1)} // yesterday
	if autoInstallDue(now, marker, 7, "05:00") {
		t.Fatal("expected false: last attempt only 1 day ago, interval is 7")
	}
}

func TestAutoInstallDue_StaleMarker_AfterWindow_Due(t *testing.T) {
	now := time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC)
	marker := &autoInstallMarker{LastAttemptAt: now.AddDate(0, 0, -8)}
	if !autoInstallDue(now, marker, 7, "05:00") {
		t.Fatal("expected true: last attempt 8 days ago, interval is 7, now after HH:MM")
	}
}

func TestAutoInstallDue_StaleMarker_BeforeWindow_NotDue(t *testing.T) {
	now := time.Date(2026, 7, 17, 4, 0, 0, 0, time.UTC)
	marker := &autoInstallMarker{LastAttemptAt: now.AddDate(0, 0, -8)}
	if autoInstallDue(now, marker, 7, "05:00") {
		t.Fatal("expected false: now is before today's HH:MM window")
	}
}

func TestAutoInstallDue_AntiDup_SameSlotNotFiredTwice(t *testing.T) {
	// Marker was stamped today, at/after the window — must not fire again
	// today even later in the day, regardless of interval.
	fired := time.Date(2026, 7, 17, 5, 5, 0, 0, time.UTC)
	now := time.Date(2026, 7, 17, 14, 0, 0, 0, time.UTC)
	marker := &autoInstallMarker{LastAttemptAt: fired}
	if autoInstallDue(now, marker, 1, "05:00") {
		t.Fatal("expected false: already attempted in today's window")
	}
}

func TestAutoInstallDue_InvalidHHMM(t *testing.T) {
	now := time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC)
	if autoInstallDue(now, nil, 7, "not-a-time") {
		t.Fatal("expected false for unparsable HH:MM")
	}
}

func TestAutoInstallMarker_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := &autoInstallMarker{
		LastAttemptAt: mustParseTime(t, time.RFC3339, "2026-07-17T05:00:00Z").UTC(),
		FromVersion:   "2.12.0",
		ToVersion:     "2.12.1",
	}
	if err := writeAutoInstallMarker(dir, want); err != nil {
		t.Fatalf("writeAutoInstallMarker: %v", err)
	}
	got := readAutoInstallMarker(dir)
	if got == nil {
		t.Fatal("readAutoInstallMarker returned nil after successful write")
	}
	if !got.LastAttemptAt.Equal(want.LastAttemptAt) || got.FromVersion != want.FromVersion || got.ToVersion != want.ToVersion {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", got, want)
	}
}

func TestAutoInstallMarker_MissingFile(t *testing.T) {
	dir := t.TempDir()
	if got := readAutoInstallMarker(dir); got != nil {
		t.Fatalf("expected nil for missing marker file, got %+v", got)
	}
}

func TestAutoInstallMarker_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(autoInstallMarkerPath(dir), []byte("{not json"), 0644); err != nil {
		t.Fatalf("write corrupt marker: %v", err)
	}
	if got := readAutoInstallMarker(dir); got != nil {
		t.Fatalf("expected nil for corrupt marker file, got %+v", got)
	}
}

// --- fakeSingboxUpdater: exercises the slot / catch-up / retrospective logic. ---

type fakeSingboxUpdater struct {
	installed       bool
	updateAvailable bool
	current         string
	required        string
	updateErr       error
	updateCalls     int
	// versionAfterUpdate, if set, is what UpdateStatus reports as current
	// once Update has been called at least once (simulates a real swap).
	versionAfterUpdate string
}

func (f *fakeSingboxUpdater) UpdateStatus(ctx context.Context) (installed, updateAvailable bool, current, required string) {
	cur := f.current
	if f.updateCalls > 0 && f.versionAfterUpdate != "" {
		cur = f.versionAfterUpdate
	}
	return f.installed, f.updateAvailable, cur, f.required
}

func (f *fakeSingboxUpdater) Update(ctx context.Context) error {
	f.updateCalls++
	return f.updateErr
}

// failingDownloader simulates "no network" for CheckNow so the manager
// self-update path in runAutoInstallActions reliably falls through to the
// sing-box path in tests, without ever making a real HTTP request.
func failingDownloader() Downloader {
	return &fakeDownloader{
		readAllFn: func(context.Context, downloader.Request) ([]byte, downloader.ResponseMeta, error) {
			return nil, downloader.ResponseMeta{}, errors.New("no network in test")
		},
	}
}

func newTestUpdateService(t *testing.T, su SingboxUpdater) *Service {
	t.Helper()
	dir := t.TempDir()
	dl := failingDownloader()
	return &Service{
		version:        "2.12.0",
		dataDir:        dir,
		singboxUpdater: su,
		downloader:     dl,
		changelog:      newChangelogFetcher(changelogURLForChannel(channelStable), 10*time.Minute, dl),
	}
}

func TestRunAutoInstallSlot_SingboxUpdateAvailable_WritesMarkerAndUpdates(t *testing.T) {
	su := &fakeSingboxUpdater{
		installed:          true,
		updateAvailable:    true,
		current:            "1.11.0",
		required:           "1.11.1",
		versionAfterUpdate: "1.11.1",
	}
	svc := newTestUpdateService(t, su)
	// No manager update available: CheckNow will hit the fake repo URL and
	// fail (no network in tests) — that's fine, info.Available stays false.
	svc.runAutoInstallSlotForced()

	if su.updateCalls != 1 {
		t.Fatalf("Update calls = %d, want 1", su.updateCalls)
	}
	marker := svc.readAutoInstallMarker()
	if marker == nil {
		t.Fatal("expected marker to be written")
	}
	if marker.FromVersion != "1.11.0" || marker.ToVersion != "1.11.1" {
		t.Fatalf("marker = %+v, want from=1.11.0 to=1.11.1", marker)
	}
}

func TestRunAutoInstallSlot_SingboxNoUpdateAvailable_NoMarker(t *testing.T) {
	su := &fakeSingboxUpdater{installed: true, updateAvailable: false, current: "1.11.1", required: "1.11.1"}
	svc := newTestUpdateService(t, su)
	svc.runAutoInstallSlotForced()

	if su.updateCalls != 0 {
		t.Fatalf("Update calls = %d, want 0", su.updateCalls)
	}
	if marker := svc.readAutoInstallMarker(); marker != nil {
		t.Fatalf("expected no marker, got %+v", marker)
	}
}

func TestRunAutoInstallSlot_SingboxBusy_SkipsWithoutStamp(t *testing.T) {
	su := &fakeSingboxUpdater{
		installed:       true,
		updateAvailable: true,
		current:         "1.11.0",
		required:        "1.11.1",
		updateErr:       ErrSingboxInstallInProgress,
	}
	svc := newTestUpdateService(t, su)
	svc.runAutoInstallSlotForced()

	if su.updateCalls != 1 {
		t.Fatalf("Update calls = %d, want 1", su.updateCalls)
	}
	if marker := svc.readAutoInstallMarker(); marker != nil {
		t.Fatalf("expected no marker stamped when install already in progress, got %+v", marker)
	}
}

func TestRunAutoInstallSlot_SingboxNoOp_NotJournaledAsUpdate(t *testing.T) {
	// Update() returns nil but the version never actually changed
	// (no-space / MatchesRequired no-op case) — must not be reported as
	// a successful update, but the attempt is still stamped.
	su := &fakeSingboxUpdater{
		installed:       true,
		updateAvailable: true,
		current:         "1.11.0",
		required:        "1.11.1",
		// versionAfterUpdate left empty => UpdateStatus keeps reporting "1.11.0"
	}
	svc := newTestUpdateService(t, su)
	svc.runAutoInstallSlotForced()

	marker := svc.readAutoInstallMarker()
	if marker == nil {
		t.Fatal("expected marker to be written even on no-op")
	}
	if marker.ToVersion != "1.11.1" {
		t.Fatalf("marker.ToVersion = %q, want required version stamped as attempted", marker.ToVersion)
	}
}

func TestAutoInstallStartupCatchUp_UpdatesOnceOnMismatch(t *testing.T) {
	su := &fakeSingboxUpdater{
		installed:          true,
		updateAvailable:    true,
		current:            "1.11.0",
		required:           "1.11.1",
		versionAfterUpdate: "1.11.1",
	}
	svc := newTestUpdateService(t, su)
	svc.settings = newAutoInstallTestSettings(t, true)

	svc.autoInstallStartupCatchUp(context.Background())

	if su.updateCalls != 1 {
		t.Fatalf("Update calls = %d, want exactly 1", su.updateCalls)
	}
}

func TestAutoInstallStartupCatchUp_DisabledDoesNothing(t *testing.T) {
	su := &fakeSingboxUpdater{installed: true, updateAvailable: true, current: "1.11.0", required: "1.11.1"}
	svc := newTestUpdateService(t, su)
	svc.settings = newAutoInstallTestSettings(t, false)

	svc.autoInstallStartupCatchUp(context.Background())

	if su.updateCalls != 0 {
		t.Fatalf("Update calls = %d, want 0 when auto-install disabled", su.updateCalls)
	}
}

func TestAutoInstallRetrospective_SuccessReportedOnce(t *testing.T) {
	svc := newTestUpdateService(t, nil)
	svc.settings = newAutoInstallTestSettings(t, true)
	marker := &autoInstallMarker{
		LastAttemptAt: time.Now().Add(-time.Hour),
		FromVersion:   "2.11.9",
		ToVersion:     "2.12.0", // matches svc.version
	}
	if err := writeAutoInstallMarker(svc.dataDir, marker); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	svc.autoInstallRetrospective()

	got := svc.readAutoInstallMarker()
	if got == nil || !got.Reported {
		t.Fatalf("expected marker.Reported = true after retrospective, got %+v", got)
	}

	// Second run must not re-log (Reported already true) — no panic/second
	// write side effect beyond idempotence.
	svc.autoInstallRetrospective()
	got2 := svc.readAutoInstallMarker()
	if got2 == nil || !got2.Reported {
		t.Fatalf("expected marker to remain reported, got %+v", got2)
	}
}

func TestAutoInstallRetrospective_MismatchRecentWarns(t *testing.T) {
	svc := newTestUpdateService(t, nil)
	svc.settings = newAutoInstallTestSettings(t, true)
	marker := &autoInstallMarker{
		LastAttemptAt: time.Now().Add(-time.Hour),
		FromVersion:   "2.11.9",
		ToVersion:     "2.99.0", // does not match svc.version, recent attempt
	}
	if err := writeAutoInstallMarker(svc.dataDir, marker); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	svc.autoInstallRetrospective()

	got := svc.readAutoInstallMarker()
	if got == nil || !got.Reported {
		t.Fatalf("expected marker.Reported = true after mismatch retrospective, got %+v", got)
	}
}

func newAutoInstallTestSettings(t *testing.T, enabled bool) *storage.SettingsStore {
	t.Helper()
	dir := t.TempDir()
	store := storage.NewSettingsStore(dir)
	settings, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	settings.Updates.AutoInstallEnabled = enabled
	settings.Updates.AutoInstallIntervalDays = 7
	settings.Updates.AutoInstallTime = "05:00"
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	return store
}

// runAutoInstallSlotForced runs the slot action unconditionally, bypassing
// the autoInstallDue gate (settings not wired in these unit tests) so the
// action sequencing itself can be exercised in isolation.
func (s *Service) runAutoInstallSlotForced() {
	s.runAutoInstallActions(context.Background())
}
