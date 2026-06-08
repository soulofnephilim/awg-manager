package api

import "testing"

func TestDetectSystemServerNATMode(t *testing.T) {
	cases := []struct {
		nat, static bool
		want        string
	}{
		{true, false, "full"},
		{false, true, "internet-only"},
		{false, false, "none"},
		{true, true, "full"},
	}
	for _, c := range cases {
		if got := detectSystemServerNATMode(c.nat, c.static); got != c.want {
			t.Fatalf("detect(%v,%v) = %q, want %q", c.nat, c.static, got, c.want)
		}
	}
}
