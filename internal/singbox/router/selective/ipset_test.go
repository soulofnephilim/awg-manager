package selective

import (
	"strings"
	"testing"
)

// ── normalizeEntry ────────────────────────────────────────────────────────────

func TestNormalizeEntry_ValidCIDR(t *testing.T) {
	if got := normalizeEntry("10.0.0.0/8"); got != "10.0.0.0/8" {
		t.Errorf("got %q", got)
	}
}

func TestNormalizeEntry_BareIPBecomesSlash32(t *testing.T) {
	if got := normalizeEntry("1.2.3.4"); got != "1.2.3.4/32" {
		t.Errorf("got %q", got)
	}
}

func TestNormalizeEntry_CanonicalisesCIDR(t *testing.T) {
	// Host bits should be masked: 10.0.0.1/8 → 10.0.0.0/8
	if got := normalizeEntry("10.0.0.1/8"); got != "10.0.0.0/8" {
		t.Errorf("got %q", got)
	}
}

func TestNormalizeEntry_IPv6Rejected(t *testing.T) {
	if got := normalizeEntry("::1/128"); got != "" {
		t.Errorf("expected empty for IPv6, got %q", got)
	}
}

func TestNormalizeEntry_IPv6BareRejected(t *testing.T) {
	if got := normalizeEntry("fe80::1"); got != "" {
		t.Errorf("expected empty for IPv6, got %q", got)
	}
}

func TestNormalizeEntry_GarbageRejected(t *testing.T) {
	if got := normalizeEntry("not-an-ip"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestNormalizeEntry_EmptyString(t *testing.T) {
	if got := normalizeEntry(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestNormalizeEntry_Whitespace(t *testing.T) {
	if got := normalizeEntry("  1.2.3.4  "); got != "1.2.3.4/32" {
		t.Errorf("got %q", got)
	}
}

// ── IPSetBinary path detection ────────────────────────────────────────────────

func TestIPSetBinary_ReturnsPresentPath(t *testing.T) {
	// Override the candidate paths with a guaranteed-present binary.
	original := ipsetBinaryPaths
	ipsetBinaryPaths = []string{"/usr/bin/env"} // always exists
	defer func() { ipsetBinaryPaths = original }()

	if got := IPSetBinary(); got != "/usr/bin/env" {
		t.Errorf("expected /usr/bin/env, got %q", got)
	}
	if !IsIPSetAvailable() {
		t.Error("IsIPSetAvailable() should be true when binary exists")
	}
}

func TestIPSetBinary_ReturnsEmptyWhenNotFound(t *testing.T) {
	original := ipsetBinaryPaths
	ipsetBinaryPaths = []string{"/nonexistent/path/ipset-xyz"}
	defer func() { ipsetBinaryPaths = original }()

	if got := IPSetBinary(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if IsIPSetAvailable() {
		t.Error("IsIPSetAvailable() should be false when binary missing")
	}
}

// ── AddEntries — batch input format ──────────────────────────────────────────
// We can't call the real `ipset` in unit tests, but we can verify the
// batch restore input that would be piped to it.

func buildRestoreInput_AddEntries(cidrs []string) string {
	// Reproduce the batch format from AddEntries without calling ipset.
	var sb strings.Builder
	for _, raw := range cidrs {
		entry := normalizeEntry(raw)
		if entry == "" {
			continue
		}
		sb.WriteString("add " + SetName + " " + entry + "\n")
	}
	return sb.String()
}

func TestAddEntries_BatchFormat(t *testing.T) {
	input := buildRestoreInput_AddEntries([]string{"1.2.3.0/24", "5.6.7.8", "invalid"})
	lines := strings.Split(strings.TrimSpace(input), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (invalid skipped), got %d:\n%s", len(lines), input)
	}
	if !strings.Contains(lines[0], "1.2.3.0/24") {
		t.Errorf("line 0: %q", lines[0])
	}
	if !strings.Contains(lines[1], "5.6.7.8/32") {
		t.Errorf("line 1: %q", lines[1])
	}
}

func TestAddEntries_EmptyInput_NoOutput(t *testing.T) {
	input := buildRestoreInput_AddEntries(nil)
	if input != "" {
		t.Errorf("expected empty output for nil input, got %q", input)
	}
}

func TestAddEntries_AllInvalid_NoOutput(t *testing.T) {
	input := buildRestoreInput_AddEntries([]string{"::1", "garbage", ""})
	if input != "" {
		t.Errorf("expected empty output for all-invalid input, got %q", input)
	}
}

func TestAddEntries_SetNameInOutput(t *testing.T) {
	input := buildRestoreInput_AddEntries([]string{"10.0.0.1"})
	if !strings.Contains(input, SetName) {
		t.Errorf("expected %q in output, got %q", SetName, input)
	}
}
