// Package selective implements the selective-bypass feature for the sing-box
// TProxy router: only traffic whose destination IP is listed in the
// AWGM-SELECTIVE ipset reaches sing-box; all other traffic bypasses it
// entirely (RETURN → WAN).
package selective

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sysexec "github.com/hoaxisr/awg-manager/internal/sys/exec"
	"github.com/hoaxisr/awg-manager/internal/sys/osdetect"
)

// ipsetBinaryPaths lists candidate absolute paths for the ipset binary,
// searched in order. Entware installs to /opt/sbin; system may have
// /usr/sbin or /sbin.
var ipsetBinaryPaths = []string{
	"/opt/sbin/ipset",
	"/usr/sbin/ipset",
	"/sbin/ipset",
}

// IPSetBinary returns the path to the installed ipset binary, or ""
// when not found. Scans candidate paths in preference order.
func IPSetBinary() string {
	for _, p := range ipsetBinaryPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// IsIPSetAvailable reports whether the ipset binary is present on the router.
func IsIPSetAvailable() bool {
	return IPSetBinary() != ""
}

// xtSetModuleName is the kernel module name for iptables ipset matching.
const xtSetModuleName = "xt_set"

// IsXtSetAvailable reports whether the xt_set kernel module is currently
// loaded OR available as a .ko file that can be loaded.
// NOT cached — called at status-check time, result must reflect reality
// after module load attempts.
func IsXtSetAvailable() bool {
	if isModuleLoaded(xtSetModuleName) {
		return true
	}
	kernel := osdetect.KernelRelease()
	if kernel == "" {
		return false
	}
	path := filepath.Join("/lib/modules", kernel, xtSetModuleName+".ko")
	_, err := os.Stat(path)
	return err == nil
}

// isModuleLoaded checks /proc/modules for the given module name.
// Identical to the helper in iptables.go; duplicated to keep the
// selective package self-contained and avoid an internal import cycle.
func isModuleLoaded(name string) bool {
	data, err := os.ReadFile("/proc/modules")
	if err != nil {
		return false
	}
	prefix := name + " "
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// EnsureXtSetModule attempts to load xt_set.ko via insmod. Soft-fail by
// design: if the module is built into the kernel there is no .ko file but
// it still works; a hard error here would be a false positive. The caller
// (builder) logs the warning and continues — the real failure surfaces at
// iptables-restore COMMIT time with a concrete message.
func EnsureXtSetModule(ctx context.Context) error {
	if isModuleLoaded(xtSetModuleName) {
		return nil
	}
	kernel := osdetect.KernelRelease()
	if kernel == "" {
		return nil // soft-fail: can't determine kernel version
	}
	path := filepath.Join("/lib/modules", kernel, xtSetModuleName+".ko")
	if _, err := os.Stat(path); err != nil {
		return nil // soft-fail: .ko absent → assume built-in or unavailable
	}
	_, err := sysexec.Run(ctx, "insmod", path)
	return err
}

// InstallIPSet runs `opkg install ipset` and streams output lines to
// progressFn (called once per line, nil = silent). Returns the first
// non-zero exit error, or nil on success.
func InstallIPSet(ctx context.Context, progressFn func(line string)) error {
	opkg, err := findOpkg()
	if err != nil {
		return err
	}
	// We use RunWithOptions with a long timeout — opkg downloads packages
	// and can take 30-60s on slow WAN.
	res, err := sysexec.RunWithOptions(ctx, opkg, []string{"install", "ipset"},
		sysexec.Options{Timeout: 120e9}) // 120s
	if res != nil && progressFn != nil {
		combined := res.Stdout
		if res.Stderr != "" {
			combined += res.Stderr
		}
		for _, line := range strings.Split(combined, "\n") {
			if l := strings.TrimSpace(line); l != "" {
				progressFn(l)
			}
		}
	}
	if err != nil {
		return sysexec.FormatError(res, err)
	}
	// Invalidate the cached result so subsequent IsIPSetAvailable calls
	// reflect the newly installed binary.
	resetIPSetCache()

	// Best-effort: also install conntrack so a routing change takes effect
	// immediately (existing flows get evicted) instead of waiting for old
	// connections to expire. Failure here is non-fatal — the selective guard
	// works without it, just with delayed effect on established flows.
	if !IsConntrackAvailable() {
		_ = InstallConntrackTools(ctx, progressFn)
	}
	return nil
}

// InstallConntrackTools installs the conntrack userspace binary via opkg.
// Keenetic Entware ships it as package "conntrack"; some feeds use
// "conntrack-tools" instead — we try both.
func InstallConntrackTools(ctx context.Context, progressFn func(line string)) error {
	opkg, err := findOpkg()
	if err != nil {
		return err
	}
	for _, pkg := range []string{"conntrack", "conntrack-tools"} {
		if err := opkgInstall(ctx, opkg, pkg, progressFn); err == nil {
			return nil
		}
	}
	return fmt.Errorf("opkg install conntrack: package not found in feed")
}

func opkgInstall(ctx context.Context, opkg, pkg string, progressFn func(line string)) error {
	res, err := sysexec.RunWithOptions(ctx, opkg, []string{"install", pkg},
		sysexec.Options{Timeout: 120e9})
	if res != nil && progressFn != nil {
		combined := res.Stdout
		if res.Stderr != "" {
			combined += res.Stderr
		}
		for _, line := range strings.Split(combined, "\n") {
			if l := strings.TrimSpace(line); l != "" {
				progressFn(l)
			}
		}
	}
	if err != nil {
		return sysexec.FormatError(res, err)
	}
	return nil
}

// resetIPSetCache is kept for API compatibility with tests but is now a no-op
// since IsXtSetAvailable no longer caches.
func resetIPSetCache() {}

// findOpkg returns the absolute path to opkg, or an error if not found.
func findOpkg() (string, error) {
	for _, p := range []string{"/opt/bin/opkg", "/usr/bin/opkg", "/bin/opkg"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", ErrOpkgNotFound
}
