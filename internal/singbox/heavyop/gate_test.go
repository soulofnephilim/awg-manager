package heavyop

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGateLockWithContext_AcquiresWhenFree(t *testing.T) {
	var g Gate
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := g.LockWithContext(ctx); err != nil {
		t.Fatalf("LockWithContext on free gate: %v", err)
	}
	g.Unlock()
}

func TestGateLockWithContext_DeadlineWhileHeld(t *testing.T) {
	var g Gate
	g.Lock()
	defer g.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := g.LockWithContext(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestGateLockWithContext_SucceedsAfterUnlock(t *testing.T) {
	var g Gate
	g.Lock()
	go func() {
		time.Sleep(10 * time.Millisecond)
		g.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := g.LockWithContext(ctx); err != nil {
		t.Fatalf("LockWithContext after Unlock: %v", err)
	}
	g.Unlock()
}

func TestGateTryLock(t *testing.T) {
	var g Gate
	if !g.TryLock() {
		t.Fatal("TryLock on free gate must succeed")
	}
	if g.TryLock() {
		t.Fatal("TryLock on held gate must fail")
	}
	g.Unlock()
	if !g.TryLock() {
		t.Fatal("TryLock after Unlock must succeed")
	}
	g.Unlock()
}
