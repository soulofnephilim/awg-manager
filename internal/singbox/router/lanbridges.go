package router

import (
	"context"
	"fmt"
	"os"
	"regexp"

	sysexec "github.com/hoaxisr/awg-manager/internal/sys/exec"
	sysiptables "github.com/hoaxisr/awg-manager/internal/sys/iptables"
)

// LANBridgeMark pairs a Linux bridge name with the NDMS hotspot mark
// that NDMS assigns to traffic entering on that bridge with no specific
// MAC-override. It's the mark we elevate mark=0 (no-policy) DNS to so
// NDMS's _NDM_HOTSPOT_DNSREDIR rules pick the packet up and REDIRECT it
// to its per-policy ndnproxy port — same code path that already works
// for any unmarked LAN device.
type LANBridgeMark struct {
	Bridge string // kernel bridge name, e.g. "br0"
	Mark   string // hex mark, e.g. "0xffffaab"
}

// _NDM_HOTSPOT_PREROUTING_MANGL catch-all rule shape we parse:
//
//	-A _NDM_HOTSPOT_PREROUTING_MANGL -i br0 -j MARK --set-xmark 0xffffaab/0xffffffff
//
// MAC-specific rules and CONNMARK/RETURN follow-ups don't match this
// regex (they have either `-m mac --mac-source X` before `-j MARK`, or
// `-j CONNMARK` / `-j RETURN` as the target). The catch-all `-i <iface>
// -j MARK` form is the one we want — it's the rule that fires for a
// device on that bridge when no earlier MAC-override matched.
var hotspotCatchAllRegexp = regexp.MustCompile(
	`^-A _NDM_HOTSPOT_PREROUTING_MANGL -i ([a-zA-Z0-9_-]+) -j MARK --set-xmark (0x[0-9a-fA-F]+)/0x[0-9a-fA-F]+$`,
)

// DiscoverLANBridges returns the intersection of:
//  1. interfaces that NDMS catch-all-marks in _NDM_HOTSPOT_PREROUTING_MANGL
//     (each paired with whatever mark NDMS uses for that bridge), and
//  2. real Linux bridges in /sys/class/net/*/bridge.
//
// The intersection is exactly the set of bridges where:
//   - a device CAN end up with mark=0 (via a MAC-RETURN-override that
//     fires before the catch-all sets the mark), AND
//   - NDMS has a downstream `_NDM_HOTSPOT_DNSREDIR` rule with a
//     `-m mark --mark <mark> ... -j REDIRECT --to-ports <port>` ready
//     to receive a packet we re-mark to that bridge's catch-all mark.
//
// Excludes interfaces like nwg2 (WireGuard tunnel — not a sysfs bridge,
// no MAC-override mechanism, NDMS marks 100% of its traffic so our
// mark=0 filter would never fire anyway) and sstp-bridge (real Linux
// bridge but NDMS doesn't have a _NDM_HOTSPOT_DNSREDIR rule for it).
//
// Returns empty slice (not nil, not error) when no bridges qualify;
// that means there's nothing to fall through DNS-wise and callers
// should skip the DNS-NOPOLICY install logic.
func DiscoverLANBridges(ctx context.Context) ([]LANBridgeMark, error) {
	result, err := sysexec.Run(ctx, sysiptables.Binary, "-w", "-t", "mangle",
		"-S", "_NDM_HOTSPOT_PREROUTING_MANGL")
	if err != nil || result == nil {
		// Chain doesn't exist: router has no hotspot config (fresh
		// install, no LAN policies created yet). Nothing to elevate
		// to — return empty, caller skips.
		return []LANBridgeMark{}, nil
	}

	out := make([]LANBridgeMark, 0, 4)
	seen := make(map[string]bool, 4)
	for _, line := range splitLines(result.Stdout) {
		m := hotspotCatchAllRegexp.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		iface, mark := m[1], m[2]
		if seen[iface] {
			continue
		}
		if !isLinuxBridge(iface) {
			continue
		}
		seen[iface] = true
		out = append(out, LANBridgeMark{Bridge: iface, Mark: mark})
	}
	return out, nil
}

// isLinuxBridge reports whether the named interface is a real Linux
// bridge (has a /sys/class/net/<name>/bridge directory). WireGuard
// tunnels, physical NICs, PPP, and SSTP "bridges" that NDMS marks but
// that aren't true L2 bridges return false.
func isLinuxBridge(iface string) bool {
	info, err := os.Stat(fmt.Sprintf("/sys/class/net/%s/bridge", iface))
	return err == nil && info.IsDir()
}

// equalLANBridges reports whether two []LANBridgeMark slices have the
// same (bridge, mark) pairs in the same order. Used by reconcileInstalled
// to decide whether an iptables re-install is needed when LAN-bridge
// state on the router drifts (NDMS hotspot reconfigured, bridge added/
// removed, mark reassigned to a different policy).
func equalLANBridges(a, b []LANBridgeMark) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Bridge != b[i].Bridge || a[i].Mark != b[i].Mark {
			return false
		}
	}
	return true
}

func splitLines(s string) []string {
	out := make([]string, 0, 16)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
