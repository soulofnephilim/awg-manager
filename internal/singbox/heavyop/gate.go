// Package heavyop serializes memory-heavy sing-box work (ipset rebuild, config apply).
package heavyop

import "sync"

// Gate ensures selective ipset rebuild and sing-box config apply/reload do not run concurrently.
type Gate struct {
	mu sync.Mutex
}

// Default is the process-wide gate shared by orchestrator reload and selective rebuild.
var Default Gate

// Lock blocks until no other heavy operation is running.
func (g *Gate) Lock() {
	g.mu.Lock()
}

// Unlock releases the gate.
func (g *Gate) Unlock() {
	g.mu.Unlock()
}

// TryLock reports whether the gate was acquired without blocking.
func (g *Gate) TryLock() bool {
	return g.mu.TryLock()
}
