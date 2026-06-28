package selective

import "testing"

func TestSingleHostIP(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"188.40.167.82/32", "188.40.167.82"},
		{"188.40.167.82", "188.40.167.82"},
		{" 10.0.0.1/32 ", "10.0.0.1"},
		{"142.250.0.0/15", ""}, // broader than /32 — skipped
		{"10.0.0.0/8", ""},
		{"::1/128", ""}, // IPv6 — skipped
		{"2001:db8::1", ""},
		{"not-an-ip", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := singleHostIP(c.in); got != c.want {
			t.Errorf("singleHostIP(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// When the conntrack binary is absent the flush is a no-op and reports
// available=false (so callers can surface an install hint) without erroring.
func TestFlushConntrackForCIDRs_NoBinary(t *testing.T) {
	saved := conntrackBinaryPaths
	conntrackBinaryPaths = []string{"/nonexistent/conntrack-xyz"}
	t.Cleanup(func() { conntrackBinaryPaths = saved })

	flushed, available := FlushConntrackForCIDRs(t.Context(), []string{"188.40.167.82/32"}, nil)
	if available {
		t.Error("expected available=false when binary absent")
	}
	if flushed != 0 {
		t.Errorf("expected 0 flushed, got %d", flushed)
	}
}
