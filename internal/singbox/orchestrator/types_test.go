package orchestrator

import "testing"

func TestKnownSlotsIncludesDNSRewritesBeforeRouter(t *testing.T) {
	slots := KnownSlots()
	idxRewrites, idxRouter := -1, -1
	for i, m := range slots {
		if m.Slot == SlotDNSRewrites {
			idxRewrites = i
		}
		if m.Slot == SlotRouter {
			idxRouter = i
		}
	}
	if idxRewrites < 0 {
		t.Fatal("SlotDNSRewrites not registered")
	}
	if !(idxRewrites < idxRouter) {
		t.Errorf("rewrites slot must sort before router: rewrites=%d router=%d", idxRewrites, idxRouter)
	}
}
