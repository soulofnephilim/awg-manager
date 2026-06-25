package orchestrator

import (
	"encoding/json"
	"fmt"
	"time"
)

// scheduleReload arms (or re-arms) the debounce timer. Caller MUST
// hold o.mu. Calling repeatedly within the window coalesces into one
// reload.
func (o *Orchestrator) scheduleReload() {
	if o.reloadTimer != nil {
		o.reloadTimer.Reset(reloadDebounce)
		return
	}
	o.reloadTimer = time.AfterFunc(reloadDebounce, func() {
		if err := o.Reload(); err != nil {
			o.log("error", fmt.Sprintf("orchestrator reload: %v", err))
		}
	})
}

// Reload validates the merged enabled config and applies it to sing-box:
//   - If validation fails — log + return validation error, NO process change.
//   - If validation passes:
//   - If at least one non-base slot is enabled → ensure running:
//     start if stopped, SIGHUP if running.
//   - If only base (or nothing) is enabled → stop if running.
//
// Reload may be called manually (e.g. by Apply UI button) or fired by
// the internal debounce timer. Safe for concurrent callers — internally
// serialized by mu and the reloading flag.
func (o *Orchestrator) Reload() error {
	o.mu.Lock()
	if o.reloading {
		o.mu.Unlock()
		return nil // collapse re-entrancy
	}
	o.reloading = true
	o.dirty = false
	// Defense-in-depth: strip dangling selector/urltest members and defaults
	// (a tag whose outbound was deleted from another slot) BEFORE validating.
	// sing-box check does not catch these — like composite cycles, a missing
	// selector dependency only surfaces at "start service" (FATAL), so a stale
	// reference would otherwise reach the daemon and take the whole config down.
	pruneLogs := o.pruneDanglingSelectorRefsLocked()
	res := o.validateLocked()
	if !res.Ok() {
		o.reloading = false
		o.mu.Unlock()
		for _, m := range pruneLogs {
			o.log("info", m)
		}
		msg := fmt.Sprintf("orchestrator validation failed; reload skipped: %s", res.Error())
		o.log("error", msg)
		return res
	}
	needRunning := o.hasActiveWorkLocked()
	proc := o.proc
	shouldRun := o.shouldRun
	prevHasTun := o.prevHasTun
	newHasTun := res.HasTun
	o.mu.Unlock()
	for _, m := range pruneLogs {
		o.log("info", m)
	}

	var err error
	if proc == nil {
		// Test mode or pre-wiring — nothing to do.
	} else {
		running, _ := proc.IsRunning()
		switch {
		case needRunning && !running:
			// Honour the sticky-stop intent: if the user pressed Stop,
			// shouldRun returns false and we must not resurrect the
			// daemon merely because a slot file changed. SIGHUP and
			// stop branches stay unaffected — they don't cold-start.
			if shouldRun != nil && !shouldRun() {
				o.log("info", "orchestrator: cold-start suppressed by manual-stop intent")
				break
			}
			o.log("info", "orchestrator: starting sing-box (active slots present)")
			err = proc.Start()
		case needRunning && running:
			if newHasTun != prevHasTun {
				// sing-box cannot add/remove a tun inbound via SIGHUP — the
				// tun device never gets carrier and readiness times out. A
				// presence toggle therefore requires a full restart.
				o.log("info", "orchestrator: restarting sing-box (tun inbound toggled)")
				if e := proc.Stop(); e != nil {
					o.log("warn", "orchestrator: stop before tun-restart: "+e.Error())
				}
				err = proc.Start()
			} else {
				o.log("info", "orchestrator: SIGHUP sing-box (config changed)")
				err = proc.Reload()
			}
		case !needRunning && running:
			o.log("info", "orchestrator: stopping sing-box (no active slots)")
			err = proc.Stop()
		default:
			// !needRunning && !running — nothing to do.
		}
	}

	o.mu.Lock()
	o.reloading = false
	// Record the tun presence of the config we just applied so the next
	// Reload compares against reality. Updated for every apply branch
	// (start / SIGHUP / restart / stop): after a stop newHasTun is false
	// anyway, after a fresh start prevHasTun == newHasTun.
	o.prevHasTun = newHasTun
	o.mu.Unlock()
	return err
}

// pruneDanglingSelectorRefsLocked rewrites enabled slot files in place,
// dropping any selector/urltest member or `default` that points at an
// outbound tag no slot declares. Caller MUST hold o.mu. Returns log lines
// describing what was pruned — the caller logs them AFTER releasing o.mu
// (o.log takes o.mu, so logging here would self-deadlock).
//
// A selector member is the ONLY ref sing-box check lets through (the error
// is deferred to "start service"), so this is the one place the orchestrator
// must self-heal rather than merely reject. The surviving-member guard keeps
// a selector from being emptied (sing-box rejects a memberless selector); an
// all-dangling selector is left untouched so validateLocked still reports it.
func (o *Orchestrator) pruneDanglingSelectorRefsLocked() []string {
	var logs []string
	// builtins sing-box defines implicitly (mirrors validateWith).
	known := map[string]bool{"direct": true, "block": true, "dns": true}
	for _, m := range KnownSlots() {
		if _, ok := o.slots[m.Slot]; !ok || !o.enabled[m.Slot] {
			continue
		}
		data, err := o.readActiveBytes(m.Slot)
		if err != nil || len(data) == 0 {
			continue
		}
		var c slotConfig
		if json.Unmarshal(data, &c) != nil {
			continue
		}
		for _, ob := range c.Outbounds {
			if ob.Tag != "" {
				known[ob.Tag] = true
			}
		}
	}

	for _, m := range KnownSlots() {
		meta := m
		if _, ok := o.slots[meta.Slot]; !ok || !o.enabled[meta.Slot] {
			continue
		}
		data, err := o.readActiveBytes(meta.Slot)
		if err != nil || len(data) == 0 {
			continue
		}
		var root map[string]any
		if json.Unmarshal(data, &root) != nil {
			continue
		}
		obs, ok := root["outbounds"].([]any)
		if !ok {
			continue
		}
		changed := false
		for _, v := range obs {
			ob, ok := v.(map[string]any)
			if !ok {
				continue
			}
			members, ok := ob["outbounds"].([]any)
			if !ok || len(members) == 0 {
				continue // not a selector/urltest
			}
			kept := make([]any, 0, len(members))
			var dropped []string
			for _, mv := range members {
				tag, _ := mv.(string)
				if tag == "" || known[tag] {
					kept = append(kept, mv)
					continue
				}
				dropped = append(dropped, tag)
			}
			// Never empty a selector — leave it for validateLocked to flag.
			if len(dropped) > 0 && len(kept) > 0 {
				ob["outbounds"] = kept
				changed = true
				tag, _ := ob["tag"].(string)
				logs = append(logs, fmt.Sprintf("orchestrator: pruned dangling selector members %v from %q in [%s]", dropped, tag, meta.Slot))
			}
			if def, _ := ob["default"].(string); def != "" && !known[def] {
				delete(ob, "default")
				changed = true
				tag, _ := ob["tag"].(string)
				logs = append(logs, fmt.Sprintf("orchestrator: cleared dangling selector default %q from %q in [%s]", def, tag, meta.Slot))
			}
		}
		if !changed {
			continue
		}
		out, err := json.MarshalIndent(root, "", "  ")
		if err != nil {
			continue
		}
		if err := writeAtomic(o.activePath(meta), out); err != nil {
			logs = append(logs, fmt.Sprintf("orchestrator: rewrite pruned slot [%s]: %v", meta.Slot, err))
		}
	}
	return logs
}

// hasActiveWorkLocked reports whether sing-box has anything to do
// beyond hosting base + catalog slots. Two activation paths:
//
//   - any non-AlwaysOn slot is enabled (router / deviceproxy /
//     subscriptions) — its mere presence is the signal;
//   - an AlwaysOn slot whose meta.HasContent returns true (currently
//     SlotTunnels, when the user has defined at least one sing-box
//     tunnel).
//
// AlwaysOn catalog slots without HasContent (SlotBase, SlotAwg) never
// activate the daemon on their own — they are infrastructure for
// other slots, not a reason to keep sing-box running.
//
// Caller MUST hold o.mu.
func (o *Orchestrator) hasActiveWorkLocked() bool {
	for slot, enabled := range o.enabled {
		if !enabled {
			continue
		}
		meta, ok := o.slots[slot]
		if !ok {
			continue
		}
		if meta.AlwaysOn {
			if meta.HasContent != nil && meta.HasContent() {
				return true
			}
			continue
		}
		return true
	}
	return false
}
