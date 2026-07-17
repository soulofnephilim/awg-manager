package router

import (
	"errors"
	"testing"
)

// TestApplyRoutesSlot_RetriesAfterFailedApply guards the rebuild's self-heal
// role: a byte-identical routes slot normally skips the sing-box reload, but
// after a FAILED apply the disk state says nothing about the running process
// — the next rebuild must re-apply even when nothing changed on disk.
func TestApplyRoutesSlot_RetriesAfterFailedApply(t *testing.T) {
	var calls int
	failNext := true
	a := &selectiveBuilderAdapter{
		svc: &ServiceImpl{},
		applyNow: func() error {
			calls++
			if failNext {
				return errors.New("sighup failed")
			}
			return nil
		},
	}

	// Changed slot, apply fails → sticky failure flag set.
	a.applyRoutesSlot(true)
	if calls != 1 {
		t.Fatalf("changed slot: applyNow calls = %d, want 1", calls)
	}
	if !a.lastApplyFailed.Load() {
		t.Fatal("failed apply must set lastApplyFailed")
	}

	// Identical rebuild (changed=false) after a failed apply must still apply.
	failNext = false
	a.applyRoutesSlot(false)
	if calls != 2 {
		t.Fatalf("unchanged slot after failed apply: applyNow calls = %d, want 2", calls)
	}
	if a.lastApplyFailed.Load() {
		t.Fatal("successful apply must clear lastApplyFailed")
	}

	// Once applied successfully, an unchanged slot skips the reload again.
	a.applyRoutesSlot(false)
	if calls != 2 {
		t.Fatalf("unchanged slot after success: applyNow calls = %d, want 2", calls)
	}
}
