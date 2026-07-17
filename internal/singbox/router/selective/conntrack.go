package selective

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	sysexec "github.com/hoaxisr/awg-manager/internal/sys/exec"
)

// conntrackBinaryPaths lists candidate absolute paths for the conntrack
// binary (from the conntrack-tools package), searched in preference order.
var conntrackBinaryPaths = []string{
	"/opt/sbin/conntrack",
	"/usr/sbin/conntrack",
	"/sbin/conntrack",
}

// ConntrackBinary returns the path to the conntrack binary, or "" when not
// found. The conntrack package is optional — without it the selective guard
// guard still applies to NEW connections, it just can't evict already-tracked
// ones (changes take effect only once the old flow expires).
func ConntrackBinary() string {
	for _, p := range conntrackBinaryPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// IsConntrackAvailable reports whether the conntrack binary is present.
func IsConntrackAvailable() bool { return ConntrackBinary() != "" }

// maxConntrackSweep caps the blind per-candidate sweep used only when the
// live-flow listing fails. Without the cap a geosite-scale rebuild would
// fork one conntrack -D per queued /32 — tens of thousands of processes.
const maxConntrackSweep = 512

// FlushConntrackForCIDRs evicts existing conntrack entries whose destination
// matches the given CIDRs/IPs so that flows established BEFORE the ipset was
// populated are re-evaluated against the selective guard immediately.
//
// Only /32 host entries are flushed; broader static CIDRs are skipped.
//
// Cost model: ONE `conntrack -L` fork lists the live flow destinations, then
// `-D` runs only for candidates that actually have live flows (typically a
// handful). The naive alternative — one -D fork per candidate — spawns a
// process per resolved /32, which a big rebuild turns into tens of thousands
// of forks on a 128MB MIPS router.
func FlushConntrackForCIDRs(ctx context.Context, cidrs []string, errFn func(ip, err string)) (flushed int, available bool) {
	bin := ConntrackBinary()
	if bin == "" {
		return 0, false
	}

	candidates := make([]string, 0, len(cidrs))
	seen := make(map[string]struct{}, len(cidrs))
	for _, raw := range cidrs {
		dest := conntrackDestArg(raw)
		if dest == "" {
			continue
		}
		ip := strings.TrimSuffix(dest, "/32")
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		candidates = append(candidates, ip)
	}
	if len(candidates) == 0 {
		return 0, true
	}

	targets := candidates
	if live, err := listConntrackDests(ctx, bin); err == nil {
		targets = targets[:0]
		for _, ip := range candidates {
			if _, ok := live[ip]; ok {
				targets = append(targets, ip)
			}
		}
	} else if len(targets) > maxConntrackSweep {
		// Listing failed — fall back to a bounded blind sweep so we never
		// re-introduce the fork-per-/32 storm. Remaining flows expire on
		// their own; log the truncation instead of hiding it.
		if errFn != nil {
			errFn("conntrack -L", fmt.Sprintf("listing failed (%v); sweeping first %d of %d candidates", err, maxConntrackSweep, len(targets)))
		}
		targets = targets[:maxConntrackSweep]
	}

	for _, ip := range targets {
		// `conntrack -D -d <ip>` deletes flows matching the destination.
		res, err := sysexec.Run(ctx, bin, "-D", "-d", ip)
		if err != nil {
			combined := ""
			if res != nil {
				combined = res.Stdout + res.Stderr
			}
			// "0 flow entries have been deleted" is reported on stderr with a
			// non-zero exit — not an error worth surfacing.
			if strings.Contains(combined, "0 flow entries") {
				continue
			}
			if errFn != nil {
				errFn(ip, err.Error())
			}
			continue
		}
		flushed++
	}
	return flushed, true
}

// listConntrackDests runs `conntrack -L` once and returns the set of live
// original-direction destination IPv4s.
func listConntrackDests(ctx context.Context, bin string) (map[string]struct{}, error) {
	res, err := sysexec.Run(ctx, bin, "-L")
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, fmt.Errorf("conntrack -L: empty result")
	}
	return parseConntrackDests(res.Stdout), nil
}

// parseConntrackDests extracts the first (original-direction) dst= IPv4 from
// each `conntrack -L` output line.
func parseConntrackDests(out string) map[string]struct{} {
	dests := make(map[string]struct{})
	for _, line := range strings.Split(out, "\n") {
		idx := strings.Index(line, "dst=")
		if idx < 0 {
			continue
		}
		rest := line[idx+4:]
		if end := strings.IndexByte(rest, ' '); end >= 0 {
			rest = rest[:end]
		}
		if ip := net.ParseIP(strings.TrimSpace(rest)); ip != nil && ip.To4() != nil {
			dests[ip.To4().String()] = struct{}{}
		}
	}
	return dests
}

// conntrackDestArg returns the -d argument for conntrack -D: /32 hosts only.
func conntrackDestArg(raw string) string {
	entry := normalizeEntry(raw)
	if entry == "" {
		return ""
	}
	_, ipnet, err := net.ParseCIDR(entry)
	if err != nil {
		return ""
	}
	ones, bits := ipnet.Mask.Size()
	if bits != 32 || ones != 32 {
		return ""
	}
	return entry
}

// singleHostIP returns the bare IPv4 address for a "/32" CIDR or a bare IPv4
// literal, or "" for anything broader or non-IPv4.
func singleHostIP(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if ip, ipnet, err := net.ParseCIDR(raw); err == nil {
		ones, bits := ipnet.Mask.Size()
		if bits != 32 || ones != 32 {
			return "" // broader than /32 — skip
		}
		if ip4 := ip.To4(); ip4 != nil {
			return ip4.String()
		}
		return ""
	}
	if ip := net.ParseIP(raw); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			return ip4.String()
		}
	}
	return ""
}

// conntrackHint is a stable string the builder can attach to progress when
// conntrack is missing, explaining why a routing change may lag.
const conntrackHint = "conntrack не установлен: смена маршрута применится к новым соединениям; для мгновенного эффекта после полной пересборки установите пакет (opkg install conntrack)"
