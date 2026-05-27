package api

import (
	"testing"

	"github.com/hoaxisr/awg-manager/internal/hydraroute"
)

func TestHydraRouteStatusData_MapsAllFields(t *testing.T) {
	in := hydraroute.Status{
		Installed:    true,
		Running:      false,
		Version:      "2.4.1",
		PID:          1234,
		StalePID:     5678,
		ProcessState: hydraroute.StateDead,
		LastError:    "neo restart: exit status 1",
	}

	got := hydraRouteStatusData(in)

	if got.Installed != in.Installed {
		t.Fatalf("Installed=%v want %v", got.Installed, in.Installed)
	}
	if got.Running != in.Running {
		t.Fatalf("Running=%v want %v", got.Running, in.Running)
	}
	if got.Version != in.Version {
		t.Fatalf("Version=%q want %q", got.Version, in.Version)
	}
	if got.PID != in.PID {
		t.Fatalf("PID=%d want %d", got.PID, in.PID)
	}
	if got.StalePID != in.StalePID {
		t.Fatalf("StalePID=%d want %d", got.StalePID, in.StalePID)
	}
	if got.ProcessState != string(in.ProcessState) {
		t.Fatalf("ProcessState=%q want %q", got.ProcessState, in.ProcessState)
	}
	if got.LastError != in.LastError {
		t.Fatalf("LastError=%q want %q", got.LastError, in.LastError)
	}
}
