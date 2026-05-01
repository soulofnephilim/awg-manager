package orchestrator

import (
	"context"
	"sync"
)

// ProcessController is the subset of sing-box.Process that orchestrator
// uses to manage lifecycle and reload. Full Process satisfies it.
type ProcessController interface {
	IsRunning() (bool, error)
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Reload() error // SIGHUP to running process
}

// Orchestrator is the single writer for sing-box config.d. See package
// doc. Safe for concurrent use by registered producers.
type Orchestrator struct {
	configDir string
	proc      ProcessController

	mu      sync.Mutex
	slots   map[Slot]SlotMeta
	enabled map[Slot]bool // last-known on-disk state
}

// New constructs an orchestrator rooted at configDir (typically
// /opt/etc/sing-box/config.d). It does NOT touch disk — call Bootstrap
// after construction to scan/migrate existing files.
func New(configDir string, proc ProcessController) *Orchestrator {
	return &Orchestrator{
		configDir: configDir,
		proc:      proc,
		slots:     make(map[Slot]SlotMeta),
		enabled:   make(map[Slot]bool),
	}
}

// Register adds a slot to the registry. Returns ErrSlotAlreadyRegistered
// if called twice for the same slot.
func (o *Orchestrator) Register(meta SlotMeta) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.slots[meta.Slot]; ok {
		return ErrSlotAlreadyRegistered
	}
	o.slots[meta.Slot] = meta
	// AlwaysOn slots are implicitly enabled.
	if meta.AlwaysOn {
		o.enabled[meta.Slot] = true
	}
	return nil
}

// Save writes the slot's JSON. Implementation in Task 2.
func (o *Orchestrator) Save(slot Slot, jsonBytes []byte) error {
	return nil
}

// SetEnabled toggles slot activity via rename-marker. Implementation in Task 2.
func (o *Orchestrator) SetEnabled(slot Slot, enabled bool) error {
	return nil
}

// Snapshot returns the current state of all registered slots.
// Implementation in Task 2.
func (o *Orchestrator) Snapshot() []SlotState {
	return nil
}

// Reload triggers a debounced SIGHUP. Implementation in Task 4.
func (o *Orchestrator) Reload() error {
	return nil
}
