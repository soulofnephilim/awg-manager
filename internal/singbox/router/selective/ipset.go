package selective

import (
	"context"
	"fmt"
	"net"
	"strings"

	sysexec "github.com/hoaxisr/awg-manager/internal/sys/exec"
)

const (
	// SetName is the ipset name used for the AWGM selective bypass filter.
	SetName = "AWGM-SELECTIVE"

	// setMaxElem is the maximum number of entries in the ipset.
	// hash:net on Keenetic kernels supports up to ~1M entries; 262144
	// is a safe ceiling that covers all realistic rule-set sizes without
	// consuming excessive kernel memory.
	setMaxElem = 262144
)

// ipsetBin returns the path to ipset, or an error if not available.
func ipsetBin() (string, error) {
	p := IPSetBinary()
	if p == "" {
		return "", ErrIPSetNotAvailable
	}
	return p, nil
}

// CreateSet creates the AWGM-SELECTIVE ipset (hash:net) if it does not
// already exist. Idempotent — "set with the same name already exists" is
// silently ignored.
func CreateSet(ctx context.Context) error {
	bin, err := ipsetBin()
	if err != nil {
		return err
	}
	res, err := sysexec.Run(ctx, bin,
		"create", SetName, "hash:net",
		"maxelem", fmt.Sprintf("%d", setMaxElem),
		"family", "inet",
	)
	if err != nil {
		// "set with the same name already exists" → idempotent success
		combined := ""
		if res != nil {
			combined = res.Stdout + res.Stderr
		}
		if strings.Contains(combined, "already exists") {
			return nil
		}
		return sysexec.FormatError(res, fmt.Errorf("ipset create: %w", err))
	}
	return nil
}

// DestroySet removes the AWGM-SELECTIVE ipset. Idempotent — "set does not
// exist" is silently ignored (set was never created or already cleaned up).
func DestroySet(ctx context.Context) error {
	bin, err := ipsetBin()
	if err != nil {
		return err
	}
	res, err := sysexec.Run(ctx, bin, "destroy", SetName)
	if err != nil {
		combined := ""
		if res != nil {
			combined = res.Stdout + res.Stderr
		}
		if strings.Contains(combined, "does not exist") || strings.Contains(combined, "not found") {
			return nil
		}
		return sysexec.FormatError(res, fmt.Errorf("ipset destroy: %w", err))
	}
	return nil
}

// FlushSet removes all entries from AWGM-SELECTIVE without deleting the set
// itself. Idempotent — if the set does not exist, it is created first.
func FlushSet(ctx context.Context) error {
	bin, err := ipsetBin()
	if err != nil {
		return err
	}
	res, err := sysexec.Run(ctx, bin, "flush", SetName)
	if err != nil {
		combined := ""
		if res != nil {
			combined = res.Stdout + res.Stderr
		}
		if strings.Contains(combined, "does not exist") || strings.Contains(combined, "not found") {
			// Set was not created yet — create it and return; it will be
			// populated by a subsequent AddEntries call.
			return CreateSet(ctx)
		}
		return sysexec.FormatError(res, fmt.Errorf("ipset flush: %w", err))
	}
	return nil
}

// SetExists reports whether the AWGM-SELECTIVE ipset currently exists in
// the kernel. Uses `ipset list -name` which is fast (no entry output).
func SetExists(ctx context.Context) bool {
	bin, err := ipsetBin()
	if err != nil {
		return false
	}
	res, err := sysexec.Run(ctx, bin, "list", "-name")
	if err != nil || res == nil {
		return false
	}
	for _, line := range strings.Split(res.Stdout, "\n") {
		if strings.TrimSpace(line) == SetName {
			return true
		}
	}
	return false
}

// EntryCount returns the number of entries in the AWGM-SELECTIVE ipset,
// or 0 if the set does not exist or the count cannot be determined.
func EntryCount(ctx context.Context) int {
	bin, err := ipsetBin()
	if err != nil {
		return 0
	}
	res, err := sysexec.Run(ctx, bin, "list", SetName)
	if err != nil || res == nil {
		return 0
	}
	count := 0
	inMembers := false
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "Members:" {
			inMembers = true
			continue
		}
		if inMembers && line != "" {
			count++
		}
	}
	return count
}

// ListEntries returns every member CIDR currently in AWGM-SELECTIVE.
// Returns nil when the set does not exist.
func ListEntries(ctx context.Context) ([]string, error) {
	bin, err := ipsetBin()
	if err != nil {
		return nil, err
	}
	res, err := sysexec.Run(ctx, bin, "list", SetName)
	if err != nil || res == nil {
		return nil, err
	}
	var out []string
	inMembers := false
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "Members:" {
			inMembers = true
			continue
		}
		if !inMembers || line == "" {
			continue
		}
		if entry := normalizeEntry(line); entry != "" {
			out = append(out, entry)
		}
	}
	return out, nil
}

// AddEntries bulk-adds CIDRs/IPs to the AWGM-SELECTIVE set using
// `ipset restore` piped input for efficiency (one syscall for all entries
// instead of N `ipset add` calls). Each entry is validated as a CIDR or
// bare IPv4 before submission; invalid entries are silently skipped.
//
// The set must already exist (call CreateSet or FlushSet first).
func AddEntries(ctx context.Context, cidrs []string) error {
	return chunkedAddToSet(ctx, SetName, cidrs)
}

// normalizeEntry canonicalises a CIDR or bare IPv4 address for ipset.
// Returns "" for anything that is not a valid IPv4 address or CIDR.
// IPv6 is not supported — sing-box TProxy on Keenetic is IPv4-only.
func normalizeEntry(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Try CIDR first.
	if _, ipnet, err := net.ParseCIDR(raw); err == nil {
		if ipnet.IP.To4() == nil {
			return "" // IPv6 — skip
		}
		return ipnet.String() // canonical form (e.g. "10.0.0.0/8")
	}
	// Try bare IP.
	if ip := net.ParseIP(raw); ip != nil {
		if ip.To4() == nil {
			return "" // IPv6 — skip
		}
		return ip.To4().String() + "/32"
	}
	return ""
}
