package selective

import (
	"context"
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

// FlushConntrackForCIDRs evicts existing conntrack entries whose destination
// matches the given CIDRs/IPs so that flows established BEFORE the ipset was
// populated are re-evaluated against the selective guard immediately.
//
// Only /32 host entries are flushed; broader static CIDRs are skipped.
func FlushConntrackForCIDRs(ctx context.Context, cidrs []string, errFn func(ip, err string)) (flushed int, available bool) {
	bin := ConntrackBinary()
	if bin == "" {
		return 0, false
	}
	for _, raw := range cidrs {
		dest := conntrackDestArg(raw)
		if dest == "" {
			continue
		}
		// `conntrack -D -d <ip[/mask]>` deletes flows matching the destination.
		res, err := sysexec.Run(ctx, bin, "-D", "-d", dest)
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
				errFn(dest, err.Error())
			}
			continue
		}
		flushed++
	}
	return flushed, true
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
