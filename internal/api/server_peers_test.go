package api

import (
	"testing"

	"github.com/hoaxisr/awg-manager/internal/ndms"
)

func TestPeerTunnelIPInUse(t *testing.T) {
	server := &ndms.WireguardServer{
		Peers: []ndms.WireguardServerPeer{
			{PublicKey: "A=", AllowedIPs: []string{"10.0.0.20/32"}},
			{PublicKey: "B=", AllowedIPs: []string{"10.0.0.3/32"}},
		},
	}
	tests := []struct {
		name     string
		tunnelIP string
		want     bool
	}{
		// Regression: "10.0.0.2" is a string prefix of "10.0.0.20" but a
		// distinct host — the old HasPrefix check wrongly reported it in use.
		{"prefix-overlap is free", "10.0.0.2/32", false},
		{"exact match is in use", "10.0.0.20/32", true},
		{"other exact match in use", "10.0.0.3/32", true},
		{"unrelated free", "10.0.0.99/32", false},
		{"invalid input is not in use", "garbage", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := peerTunnelIPInUse(server, tt.tunnelIP); got != tt.want {
				t.Errorf("peerTunnelIPInUse(%q) = %v, want %v", tt.tunnelIP, got, tt.want)
			}
		})
	}
}
