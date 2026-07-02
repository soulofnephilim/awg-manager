package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hoaxisr/awg-manager/internal/tunnel"
)

// Issue #426: the per-tunnel execution lock must be bounded. A wedged
// operation used to make every subsequent start/stop/replace request block
// on a bare mutex forever — the UI showed «Операция уже выполняется» until
// the daemon was restarted.
func TestLockTunnel_BusyFailsFastOnCtxCancel(t *testing.T) {
	o := &Orchestrator{}

	if err := o.lockTunnel(context.Background(), "tun_a"); err != nil {
		t.Fatalf("first lock: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := o.lockTunnel(ctx, "tun_a")
	if !errors.Is(err, tunnel.ErrOperationInProgress) {
		t.Fatalf("busy lock: err = %v, want ErrOperationInProgress", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("busy lock waited %s — must give up with the caller's ctx", elapsed)
	}

	// Release → the tunnel is lockable again (no leaked state).
	o.unlockTunnel("tun_a")
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := o.lockTunnel(ctx2, "tun_a"); err != nil {
		t.Fatalf("relock after unlock: %v", err)
	}
	o.unlockTunnel("tun_a")
}

// Different tunnels never contend with each other.
func TestLockTunnel_IndependentPerTunnel(t *testing.T) {
	o := &Orchestrator{}
	if err := o.lockTunnel(context.Background(), "tun_a"); err != nil {
		t.Fatalf("lock a: %v", err)
	}
	defer o.unlockTunnel("tun_a")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := o.lockTunnel(ctx, "tun_b"); err != nil {
		t.Fatalf("lock b must not contend with a: %v", err)
	}
	o.unlockTunnel("tun_b")
}

// Double-unlock (or unlock after cleanupTunnelLock) must be a harmless no-op.
func TestUnlockTunnel_Idempotent(t *testing.T) {
	o := &Orchestrator{}
	if err := o.lockTunnel(context.Background(), "tun_a"); err != nil {
		t.Fatalf("lock: %v", err)
	}
	o.unlockTunnel("tun_a")
	o.unlockTunnel("tun_a") // must not panic or corrupt the semaphore
	if err := o.lockTunnel(context.Background(), "tun_a"); err != nil {
		t.Fatalf("relock: %v", err)
	}
	o.unlockTunnel("tun_a")
}
