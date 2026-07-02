package selective

// BuildIpsetEntries merges static CIDRs and per-domain resolved IPs into the
// deduplicated list passed to ipset restore. Host routes stay /32 — widening
// to /24 pulled unrelated neighbours into sing-box and broke selective bypass.
func BuildIpsetEntries(static []string, domainResults []DomainResolveResult) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(raw string) {
		entry := normalizeEntry(raw)
		if entry == "" {
			return
		}
		if _, ok := seen[entry]; ok {
			return
		}
		seen[entry] = struct{}{}
		out = append(out, entry)
	}
	for _, c := range static {
		add(c)
	}
	for _, dr := range domainResults {
		if dr.Error != "" {
			continue
		}
		for _, ip := range dr.IPs {
			add(ip)
		}
	}
	return out
}
