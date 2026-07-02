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

func TestParseConntrackDests(t *testing.T) {
	out := "tcp      6 431999 ESTABLISHED src=192.168.1.10 dst=1.2.3.4 sport=51000 dport=443 src=1.2.3.4 dst=203.0.113.7 sport=443 dport=51000 [ASSURED] mark=0 use=1\n" +
		"udp      17 30 src=192.168.1.11 dst=8.8.8.8 sport=40000 dport=53 [UNREPLIED] src=8.8.8.8 dst=192.168.1.11\n" +
		"conntrack v1.4.7 (conntrack-tools): 2 flow entries have been shown.\n" +
		"garbage line without dest\n"
	dests := parseConntrackDests(out)
	// Only the FIRST (original-direction) dst per line counts.
	for _, want := range []string{"1.2.3.4", "8.8.8.8"} {
		if _, ok := dests[want]; !ok {
			t.Errorf("missing dst %s, got %v", want, dests)
		}
	}
	if _, ok := dests["203.0.113.7"]; ok {
		t.Errorf("reply-direction dst must not be collected, got %v", dests)
	}
	if len(dests) != 2 {
		t.Errorf("expected 2 dests, got %v", dests)
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
