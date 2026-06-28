package selective

import (
	"net/netip"
	"strings"
)

// cdnEdgePrefixStrings lists IPv4 prefixes commonly used by anycast CDNs
// (Cloudflare, Fastly, Akamai edge, AWS CloudFront frontends). Used only to
// decide whether to run an extra-aggressive DNS sampling pass — we never add
// whole provider ranges to ipset.
var cdnEdgePrefixStrings = []string{
	// Cloudflare (published https://www.cloudflare.com/ips-v4)
	"173.245.48.0/20",
	"103.21.244.0/22",
	"103.22.200.0/22",
	"103.31.4.0/22",
	"141.101.64.0/18",
	"108.162.192.0/18",
	"190.93.240.0/20",
	"188.114.96.0/20",
	"197.234.240.0/22",
	"198.41.128.0/17",
	"162.158.0.0/15",
	"104.16.0.0/13",
	"104.24.0.0/14",
	"172.64.0.0/13",
	"131.0.72.0/22",
	// Fastly (common anycast blocks)
	"151.101.0.0/16",
	"199.232.0.0/16",
	"199.27.128.0/21",
	// Akamai (representative edge ranges)
	"23.32.0.0/11",
	"23.64.0.0/14",
	"2.16.0.0/13",
	// AWS CloudFront / Global Accelerator (frequently seen on CDN CNAMEs)
	"52.0.0.0/11",
	"54.192.0.0/16",
	"99.84.0.0/16",
	"35.71.0.0/16",
	"35.72.0.0/16",
}

var cdnEdgePrefixes []netip.Prefix

func init() {
	for _, s := range cdnEdgePrefixStrings {
		if p, err := netip.ParsePrefix(s); err == nil {
			cdnEdgePrefixes = append(cdnEdgePrefixes, p)
		}
	}
}

func isCDNEdgeIP(ip string) bool {
	ip = strings.TrimSuffix(strings.TrimSpace(ip), "/32")
	addr, err := netip.ParseAddr(ip)
	if err != nil || !addr.Is4() {
		return false
	}
	for _, p := range cdnEdgePrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func containsCDNEdgeIP(cidrs []string) bool {
	for _, c := range cidrs {
		if isCDNEdgeIP(c) {
			return true
		}
	}
	return false
}
