package selective

import "testing"

func TestIsCDNEdgeIP_Cloudflare(t *testing.T) {
	if !isCDNEdgeIP("104.26.2.214") {
		t.Fatal("expected Cloudflare IP to match")
	}
	if !isCDNEdgeIP("108.162.192.147/32") {
		t.Fatal("expected Cloudflare /32 to match")
	}
}

func TestIsCDNEdgeIP_NonCDN(t *testing.T) {
	if isCDNEdgeIP("198.58.111.63") {
		t.Fatal("Linode-like IP should not match CDN prefixes")
	}
}

func TestContainsCDNEdgeIP(t *testing.T) {
	if !containsCDNEdgeIP([]string{"1.2.3.4/32", "172.67.71.137/32"}) {
		t.Fatal("expected contains CDN")
	}
}
