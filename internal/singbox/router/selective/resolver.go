package selective

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/hoaxisr/awg-manager/internal/sys/httpclient"
)

// maxConcurrentResolves caps the number of simultaneous DNS goroutines
// to avoid flooding the DNS server on large domain lists.
const maxConcurrentResolves = 10

// dnsQueryTimeout is the per-UDP-query budget. Keenetic often blocks public
// resolvers — 5s × many hosts × many servers made rebuilds feel hung.
const dnsQueryTimeout = 2 * time.Second

// dnsResolveRounds is how many parallel sampling rounds per host on local DNS.
const dnsResolveRounds = 2

// dnsResolveRoundsCDN is extra rounds on local+public DNS for CDN hosts only.
const dnsResolveRoundsCDN = 3

// maxHostsPerMatcher caps hostname expansion (suffix probes + CNAME targets)
// so a single rule cannot explode rebuild time.
const maxHostsPerMatcher = 48

// publicDNSFallbacks are always unioned into the rebuild resolver set so we
// sample more CDN edge addresses than a single upstream would return.
var publicDNSFallbacks = []string{
	"1.1.1.1",
	"8.8.8.8",
	"9.9.9.9",
	"208.67.222.222",
	"77.88.8.8",
}

// DNSServerSource is the interface the resolver uses to pull fallback DNS
// server addresses from the NDMS router. DNSProxyStatusStore satisfies it.
type DNSServerSource interface {
	// List returns the raw bytes of /show/dns-proxy response.
	List(ctx context.Context) ([]byte, error)
}

// dnsProxyStatusUpstream is the partial shape of one upstream entry in
// the /show/dns-proxy JSON response. We only need the address field.
type dnsProxyStatusUpstream struct {
	Address string `json:"address"`
}

// dnsProxyStatusProxy is the partial shape of one dns-proxy group in
// the /show/dns-proxy JSON response.
type dnsProxyStatusProxy struct {
	Upstreams []dnsProxyStatusUpstream `json:"upstreams"`
}

// dnsProxyStatusResponse is the root of the /show/dns-proxy JSON. The
// router can return either an array of proxy groups or a single group.
// We handle both.
type dnsProxyStatusResponse struct {
	// When the router returns an array at the top level.
	groups []dnsProxyStatusProxy
}

// BuildDNSServers returns the deduplicated union of DNS resolvers used when
// populating the selective ipset:
//  1. sing-box router DNS servers (udp/tls/https/h3/quic with a server field)
//  2. NDMS router upstreams (/show/dns-proxy)
//  3. publicDNSFallbacks (1.1.1.1, 8.8.8.8, …)
//
// Unioning every source matters for CDN domains: each resolver may return a
// different subset of anycast A records for the same hostname.
func BuildDNSServers(ctx context.Context, singboxDNSServers []SingboxDNSServer, ndmsSource DNSServerSource) []string {
	var seeds []string

	for _, srv := range singboxDNSServers {
		t := strings.ToLower(srv.Type)
		if t != "udp" && t != "tls" && t != "https" && t != "h3" && t != "quic" {
			continue
		}
		if srv.Server == "" {
			continue
		}
		if addr := extractHost(srv.Server); addr != "" {
			seeds = append(seeds, addr)
		}
	}

	if ndmsSource != nil {
		// Best-effort: NDMS RCI can stall on a loaded router (cold boot). The
		// discovery only widens the resolver union — bound it so a hung
		// /show/dns-proxy call cannot delay the whole rebuild.
		ndmsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		raw, err := ndmsSource.List(ndmsCtx)
		cancel()
		if err == nil && len(raw) > 0 {
			seeds = append(seeds, parseNDMSUpstreamAddresses(raw)...)
		}
	}

	return augmentDNSServers(seeds)
}

// partitionDNSServers splits the rebuild resolver list into router/sing-box
// upstreams vs public fallbacks. Public resolvers are queried only during the
// CDN extra pass — they are often unreachable from LAN and must not block the
// initial resolve.
func partitionDNSServers(servers []string) (primary, public []string) {
	pub := make(map[string]struct{}, len(publicDNSFallbacks))
	for _, p := range publicDNSFallbacks {
		pub[p] = struct{}{}
	}
	for _, s := range servers {
		s = extractHost(s)
		if s == "" {
			continue
		}
		if _, ok := pub[s]; ok {
			public = append(public, s)
		} else {
			primary = append(primary, s)
		}
	}
	if len(primary) == 0 {
		return public, nil
	}
	return primary, public
}

func augmentDNSServers(servers []string) []string {
	seen := make(map[string]struct{}, len(servers)+len(publicDNSFallbacks))
	var out []string
	add := func(addr string) {
		addr = extractHost(addr)
		if addr == "" {
			return
		}
		if _, ok := seen[addr]; ok {
			return
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	for _, s := range servers {
		add(s)
	}
	for _, s := range publicDNSFallbacks {
		add(s)
	}
	return out
}

// SingboxDNSServer is a minimal view of a DNSServer from the router config,
// carrying only the fields needed by BuildDNSServers. Exported so the router
// adapter can pass DNS server list without importing router types.
type SingboxDNSServer struct {
	Tag    string
	Type   string
	Server string
}

// suffixResolvePrefixes are prepended to a domain_suffix matcher when building
// DNS query hostnames. CDN-backed sites often serve content from these
// subdomains while the rule only names the apex or suffix.
var suffixResolvePrefixes = []string{
	"www.",
	"m.",
	"cdn.",
	"static.",
	"media.",
	"api.",
	"img.",
	"images.",
	"image.",
	"assets.",
	"edge.",
	"cache.",
	"origin.",
	"download.",
	"files.",
	"content.",
	"lb.",
	"mobile.",
}

// minimalSuffixProbes is the reduced probe set used when a rebuild carries
// more suffix matchers than fullProbeSuffixBudget. Speculative subdomain
// probes are the dominant query multiplier: 18 prefixes × thousands of
// geosite entries × servers × rounds is hundreds of thousands of UDP
// queries per rebuild. Hand-written rules (few matchers) keep the full set.
var minimalSuffixProbes = []string{"www."}

// fullProbeSuffixBudget is the max number of suffix matchers in one resolve
// batch that still get the full suffixResolvePrefixes expansion.
const fullProbeSuffixBudget = 64

// fullProbeFlags returns the per-query probe mode: the FIRST
// fullProbeSuffixBudget suffix matchers keep the full expansion, the rest
// fall back to minimalSuffixProbes. Collection order is rules first, then
// rule-sets (StreamCollectFromRules), so a geosite import consumes the
// budget AFTER the user's hand-written rules — importing a huge category
// does not demote the hand-written matchers to minimal probing.
// Exact-domain queries always resolve a single host, so the flag is moot
// for them and they do not consume the budget.
func fullProbeFlags(queries []DomainQuery) []bool {
	out := make([]bool, len(queries))
	budget := fullProbeSuffixBudget
	for i, q := range queries {
		if q.Kind == KindDomain {
			out[i] = true
			continue
		}
		if budget > 0 {
			out[i] = true
			budget--
		}
	}
	return out
}

// expandQueryHosts returns the hostnames to resolve for a rule matcher.
// Exact domain rules query only the matcher; suffix rules also probe common
// CDN / mobile subdomains so more of the site's address pool lands in ipset.
// fullProbes selects between the full prefix set and minimalSuffixProbes.
func expandQueryHosts(matcher string, kind DomainKind, fullProbes bool) []string {
	matcher = strings.TrimSpace(strings.ToLower(matcher))
	if matcher == "" {
		return nil
	}
	if kind == KindDomain {
		return []string{matcher}
	}

	seen := make(map[string]struct{})
	var out []string
	add := func(h string) {
		h = strings.TrimSpace(strings.ToLower(h))
		if h == "" {
			return
		}
		if _, ok := seen[h]; ok {
			return
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}

	add(matcher)
	probes := suffixResolvePrefixes
	if !fullProbes {
		probes = minimalSuffixProbes
	}
	for _, p := range probes {
		add(p + matcher)
	}
	return out
}

// ResolveProgressFn is called after each domain matcher finishes resolving.
type ResolveProgressFn func(done, total int, matcher string)

// ResolveHostProgressFn is called while resolving expanded hosts inside one matcher.
type ResolveHostProgressFn func(matcher, host string, hostIndex, hostTotal int)

// resolveOneQuery resolves all expanded hosts of one matcher. onHostResolved
// (optional) вызывается после каждого завершённого DNS-резолва хоста — в том
// числе в CDN-допроходе: один матчер легально резолвится минуты
// (maxHostsPerMatcher хостов × раунды × dnsQueryTimeout), и stall guard
// пересборки должен видеть прогресс внутри матчера, а не только между ними.
func resolveOneQuery(ctx context.Context, query DomainQuery, dnsServers []string, errFn func(domain, err string), hostProgressFn ResolveHostProgressFn, fullProbes bool, onHostResolved func()) DomainResolveResult {
	primary, public := partitionDNSServers(dnsServers)
	hosts := discoverQueryHosts(ctx, query.Matcher, query.Kind, primary, fullProbes)
	hostDone := func() {
		if onHostResolved != nil {
			onHostResolved()
		}
	}
	seen := make(map[string]struct{})
	var ips []string
	var ipsMu sync.Mutex
	add := func(list []string) {
		ipsMu.Lock()
		defer ipsMu.Unlock()
		for _, ip := range list {
			cidr := ip
			if !strings.Contains(cidr, "/") {
				cidr = ip + "/32"
			}
			if _, ok := seen[cidr]; ok {
				continue
			}
			seen[cidr] = struct{}{}
			ips = append(ips, cidr)
		}
	}

	type hostResolve struct {
		ips  []string
		host string
		err  error
	}
	resolved := make([]hostResolve, len(hosts))
	var wg sync.WaitGroup
	hostSem := make(chan struct{}, 6)
	for i, host := range hosts {
		wg.Add(1)
		go func(i int, host string) {
			defer wg.Done()
			hostSem <- struct{}{}
			defer func() { <-hostSem }()
			if hostProgressFn != nil {
				hostProgressFn(query.Matcher, host, i+1, len(hosts))
			}
			r, err := resolveHostIPv4Aggressive(ctx, host, primary, dnsResolveRounds, true)
			resolved[i] = hostResolve{ips: r, host: host, err: err}
			hostDone()
		}(i, host)
	}
	wg.Wait()

	var lastErr string
	var cdnHosts []string
	for _, r := range resolved {
		if r.err != nil {
			lastErr = r.err.Error()
			continue
		}
		add(r.ips)
		if containsCDNEdgeIP(r.ips) {
			cdnHosts = append(cdnHosts, r.host)
		}
	}

	if len(cdnHosts) > 0 && len(public) > 0 {
		cdnServers := append(append([]string(nil), primary...), public...)
		var cdnWg sync.WaitGroup
		cdnSem := make(chan struct{}, 4)
		for _, host := range cdnHosts {
			cdnWg.Add(1)
			go func(host string) {
				defer cdnWg.Done()
				cdnSem <- struct{}{}
				defer func() { <-cdnSem }()
				r, err := resolveHostIPv4Aggressive(ctx, host, cdnServers, dnsResolveRoundsCDN, false)
				hostDone()
				if err != nil {
					return
				}
				add(r)
			}(host)
		}
		cdnWg.Wait()
	}

	out := DomainResolveResult{
		Matcher:    query.Matcher,
		Kind:       string(query.Kind),
		QueryHosts: hosts,
		IPs:        ips,
		CDN:        containsCDNEdgeIP(ips),
		Outbound:   query.Outbound,
	}
	if len(ips) == 0 {
		out.Error = lastErr
		if out.Error == "" {
			out.Error = "no A records"
		}
		if errFn != nil {
			errFn(query.Matcher, out.Error)
		}
	}
	return out
}

// discoverQueryHosts expands suffix probes and appends CNAME targets from the
// first local resolver only (CNAME discovery is best-effort, not worth N×timeout).
func discoverQueryHosts(ctx context.Context, matcher string, kind DomainKind, primaryDNS []string, fullProbes bool) []string {
	seed := expandQueryHosts(matcher, kind, fullProbes)
	seen := make(map[string]struct{}, len(seed))
	out := make([]string, 0, len(seed))
	add := func(h string) {
		h = strings.TrimSpace(strings.ToLower(strings.TrimSuffix(h, ".")))
		if h == "" || len(out) >= maxHostsPerMatcher {
			return
		}
		if _, ok := seen[h]; ok {
			return
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	for _, h := range seed {
		add(h)
	}
	for i := 0; i < len(out); i++ {
		if len(out) >= maxHostsPerMatcher {
			break
		}
		host := out[i]
		if len(primaryDNS) == 0 {
			break
		}
		cctx, cancel := context.WithTimeout(ctx, dnsQueryTimeout)
		targets, err := httpclient.LookupCNAMETargetsViaDNS(cctx, host, primaryDNS[0], "")
		cancel()
		if err != nil {
			continue
		}
		for _, cn := range targets {
			add(cn)
		}
	}
	return out
}

// resolveHostIPv4Aggressive unions A records from servers across several
// parallel sampling rounds. When earlyExit is true, stops after the first
// round that returned any address (fast path for non-CDN hosts).
func resolveHostIPv4Aggressive(ctx context.Context, host string, servers []string, rounds int, earlyExit bool) ([]string, error) {
	if rounds < 1 {
		rounds = 1
	}
	seen := make(map[string]struct{})
	var out []string
	var outMu sync.Mutex
	addBare := func(ips []string) {
		outMu.Lock()
		defer outMu.Unlock()
		for _, ip := range ips {
			cidr := ip
			if !strings.Contains(cidr, "/") {
				cidr = ip + "/32"
			}
			if _, ok := seen[cidr]; ok {
				continue
			}
			seen[cidr] = struct{}{}
			out = append(out, cidr)
		}
	}

	var customErr, sysErr error

	for round := 0; round < rounds; round++ {
		outMu.Lock()
		before := len(out)
		outMu.Unlock()
		var wg sync.WaitGroup
		var errMu sync.Mutex

		if len(servers) > 0 {
			for _, srv := range servers {
				srv := strings.TrimSpace(srv)
				if srv == "" {
					continue
				}
				wg.Add(1)
				go func(srv string) {
					defer wg.Done()
					qctx, cancel := context.WithTimeout(ctx, dnsQueryTimeout)
					defer cancel()
					ips, err := httpclient.LookupAllIPv4ForBind(qctx, host, []string{srv}, "")
					if err != nil {
						errMu.Lock()
						if customErr == nil {
							customErr = err
						}
						errMu.Unlock()
						return
					}
					bare := make([]string, 0, len(ips))
					for _, ip := range ips {
						bare = append(bare, strings.TrimSuffix(ip, "/32"))
					}
					addBare(bare)
				}(srv)
			}
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			qctx, cancel := context.WithTimeout(ctx, dnsQueryTimeout)
			defer cancel()
			addrs, err := net.DefaultResolver.LookupHost(qctx, host)
			if err != nil {
				errMu.Lock()
				if round == 0 && sysErr == nil {
					sysErr = err
				}
				errMu.Unlock()
				return
			}
			var v4 []string
			for _, a := range addrs {
				if ip := net.ParseIP(a); ip != nil && ip.To4() != nil {
					v4 = append(v4, ip.To4().String())
				}
			}
			addBare(v4)
		}()
		wg.Wait()

		outMu.Lock()
		gotNew := len(out) > before
		outMu.Unlock()
		if earlyExit && gotNew {
			break
		}
	}

	if len(out) == 0 {
		switch {
		case customErr != nil && sysErr != nil:
			return nil, fmt.Errorf("%s; system: %s", customErr.Error(), sysErr.Error())
		case customErr != nil:
			return nil, customErr
		case sysErr != nil:
			return nil, sysErr
		default:
			return nil, fmt.Errorf("no A records")
		}
	}
	return out, nil
}

// extractHost strips port and protocol scheme from a DNS server address,
// returning a bare host (IP or hostname). Returns "" for empty or invalid input.
//
// Examples:
//
//	"8.8.8.8"         → "8.8.8.8"
//	"8.8.8.8:53"      → "8.8.8.8"
//	"tls://1.1.1.1"   → "1.1.1.1"
//	"https://dns.example.com/dns-query" → "dns.example.com"
func extractHost(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Strip scheme.
	if idx := strings.Index(s, "://"); idx >= 0 {
		s = s[idx+3:]
	}
	// Strip path.
	if idx := strings.Index(s, "/"); idx >= 0 {
		s = s[:idx]
	}
	// Strip port.
	host, _, err := net.SplitHostPort(s)
	if err == nil {
		return host
	}
	// No port — use as-is.
	return s
}

// parseNDMSUpstreamAddresses parses the raw /show/dns-proxy JSON response
// and extracts all upstream IP addresses. Returns nil on parse failure.
func parseNDMSUpstreamAddresses(raw []byte) []string {
	raw = trimBOM(raw)
	if len(raw) == 0 {
		return nil
	}

	var addrs []string
	seen := make(map[string]struct{})

	addUpstream := func(addr string) {
		host := extractHost(addr)
		if host == "" {
			return
		}
		if ip := net.ParseIP(host); ip == nil || ip.To4() == nil {
			return // skip non-IPv4 upstream addresses
		}
		if _, ok := seen[host]; ok {
			return
		}
		seen[host] = struct{}{}
		addrs = append(addrs, host)
	}

	// Try array of proxy groups: [{upstreams:[{address:"..."},...]},...].
	var proxies []dnsProxyStatusProxy
	if err := json.Unmarshal(raw, &proxies); err == nil {
		for _, p := range proxies {
			for _, u := range p.Upstreams {
				addUpstream(u.Address)
			}
		}
		return addrs
	}

	// Try single proxy group: {upstreams:[{address:"..."},...], ...}.
	var single dnsProxyStatusProxy
	if err := json.Unmarshal(raw, &single); err == nil && len(single.Upstreams) > 0 {
		for _, u := range single.Upstreams {
			addUpstream(u.Address)
		}
		return addrs
	}

	return nil
}

// trimBOM strips a UTF-8 BOM if present (some NDMS endpoints include it).
func trimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}
