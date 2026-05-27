package api

import (
	"testing"
	"time"

	"github.com/hoaxisr/awg-manager/internal/tunnel"
)

func TestOverlayPendingStatus(t *testing.T) {
	now := time.Unix(2000, 0)
	future := now.Add(10 * time.Second)
	past := now.Add(-10 * time.Second)

	cases := []struct {
		name      string
		rawState  tunnel.State
		backend   string
		quiescent time.Time
		want      string
	}{
		{"nwg broken, no bring-up yet -> needs_start", tunnel.StateBroken, "nativewg", time.Time{}, "needs_start"},
		{"nwg broken, within window -> starting", tunnel.StateBroken, "nativewg", future, "starting"},
		{"nwg broken, window elapsed -> broken", tunnel.StateBroken, "nativewg", past, "broken"},
		{"kernel broken -> broken (untouched)", tunnel.StateBroken, "kernel", time.Time{}, "broken"},
		{"nwg running -> running (untouched)", tunnel.StateRunning, "nativewg", time.Time{}, "running"},
		{"nwg starting -> starting (untouched)", tunnel.StateStarting, "nativewg", time.Time{}, "starting"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := overlayPendingStatus(tc.rawState, tc.backend, tc.quiescent, now)
			if got != tc.want {
				t.Fatalf("overlayPendingStatus(%v, %q, q, now) = %q, want %q",
					tc.rawState, tc.backend, got, tc.want)
			}
		})
	}
}
