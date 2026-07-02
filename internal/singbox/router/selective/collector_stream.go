package selective

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hoaxisr/awg-manager/internal/hydraroute"
)

// CollectSink receives matchers during a streaming collection pass.
type CollectSink struct {
	OnStaticCIDR  func(cidr string) error
	OnDomainQuery func(q DomainQuery) error
}

// GeoPaths lists on-disk geosite/geoip database files (HydraRoute / hrneo).
type GeoPaths struct {
	GeoSite []string
	GeoIP   []string
}

// RuleSetJSONOpener resolves a rule-set ref to a streamable JSON file path.
// cleanup must be called when done (may be noop).
type RuleSetJSONOpener func(ctx context.Context, ref RuleSetRef) (path string, cleanup func(), err error)

// StreamCollectFromRules walks proxy rules and rule sets, invoking sink callbacks
// without accumulating full CollectResult slices in memory.
func StreamCollectFromRules(
	ctx context.Context,
	rules []RuleJSON,
	ruleSetRefs []RuleSetRef,
	geo GeoPaths,
	openJSON RuleSetJSONOpener,
	sink CollectSink,
) []error {
	var errs []error
	seen := &deduplicator{}
	proxySetTags := make(map[string]string)

	for i := range rules {
		collectFromRuleStream(&rules[i], "", seen, sink, proxySetTags)
	}
	if len(proxySetTags) == 0 {
		return errs
	}

	refsByTag := make(map[string]RuleSetRef, len(ruleSetRefs))
	for _, ref := range ruleSetRefs {
		refsByTag[ref.Tag] = ref
	}
	for tag, outbound := range proxySetTags {
		ref, ok := refsByTag[tag]
		if !ok {
			continue
		}
		if err := streamRuleSetRef(ctx, ref, outbound, geo, openJSON, seen, &sink); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func collectFromRuleStream(r *RuleJSON, parentOutbound string, seen *deduplicator, sink CollectSink, proxySetTags map[string]string) {
	outbound := effectiveOutbound(r, parentOutbound)
	if isProxyRoute(r, outbound) {
		for _, cidr := range r.IPCIDR {
			if c := normalizeCIDR(cidr); c != "" && seen.addCIDR(c) {
				_ = sink.OnStaticCIDR(c)
			}
		}
		for _, d := range r.DomainSuffix {
			if d = cleanDomain(d); d != "" && seen.addDomainQuery(d, KindDomainSuffix, outbound) {
				_ = sink.OnDomainQuery(DomainQuery{Matcher: d, Kind: KindDomainSuffix, Outbound: outbound})
			}
		}
		for _, d := range r.Domain {
			if d = cleanDomain(d); d != "" && seen.addDomainQuery(d, KindDomain, outbound) {
				_ = sink.OnDomainQuery(DomainQuery{Matcher: d, Kind: KindDomain, Outbound: outbound})
			}
		}
		for _, tag := range r.RuleSet {
			if tag == "" {
				continue
			}
			if _, ok := proxySetTags[tag]; !ok {
				proxySetTags[tag] = outbound
			}
		}
	}
	for i := range r.Rules {
		collectFromRuleStream(&r.Rules[i], outbound, seen, sink, proxySetTags)
	}
}

func streamRuleSetRef(
	ctx context.Context,
	ref RuleSetRef,
	outbound string,
	geo GeoPaths,
	openJSON RuleSetJSONOpener,
	seen *deduplicator,
	sink *CollectSink,
) error {
	if len(ref.DatTags) > 0 && ref.DatKind != "" {
		return streamDatRuleSet(ref.DatKind, ref.DatTags, outbound, geo, seen, sink)
	}
	if len(ref.Rules) > 0 {
		for _, ruleMap := range ref.Rules {
			raw, err := json.Marshal(ruleMap)
			if err != nil {
				continue
			}
			var m map[string]json.RawMessage
			if err := json.Unmarshal(raw, &m); err != nil {
				continue
			}
			if err := streamExtractFromRuleMap(m, outbound, seen, sink); err != nil {
				return err
			}
		}
		return nil
	}
	if openJSON == nil {
		jsonPath := resolveRuleSetJSONPath(ref)
		if jsonPath == "" {
			return nil
		}
		return streamRuleSetJSONFile(jsonPath, outbound, seen, sink)
	}
	path, cleanup, err := openJSON(ctx, ref)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}
	if path == "" {
		return nil
	}
	return streamRuleSetJSONFile(path, outbound, seen, sink)
}

func streamDatRuleSet(kind string, tags []string, outbound string, geo GeoPaths, seen *deduplicator, sink *CollectSink) error {
	switch kind {
	case "geosite":
		for _, path := range geo.GeoSite {
			for _, tag := range tags {
				if err := hydraroute.StreamGeoSiteTagLines(path, tag, func(line string) error {
					return ingestGeoSiteLine(line, outbound, seen, sink)
				}); err != nil {
					if strings.Contains(err.Error(), "not found") {
						continue
					}
					return err
				}
			}
		}
	case "geoip":
		for _, path := range geo.GeoIP {
			for _, tag := range tags {
				if err := hydraroute.StreamGeoIPTagLines(path, tag, func(line string) error {
					if c := normalizeCIDR(line); c != "" && seen.addCIDR(c) {
						return sink.OnStaticCIDR(c)
					}
					return nil
				}); err != nil {
					if strings.Contains(err.Error(), "not found") {
						continue
					}
					return err
				}
			}
		}
	}
	return nil
}

func ingestGeoSiteLine(line, outbound string, seen *deduplicator, sink *CollectSink) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	if strings.HasPrefix(line, "domain_regex:") || strings.HasPrefix(line, "domain_keyword:") {
		return nil
	}
	kind := KindDomainSuffix
	matcher := line
	if strings.HasPrefix(line, "domain:") {
		kind = KindDomain
		matcher = strings.TrimSpace(strings.TrimPrefix(line, "domain:"))
	} else if strings.HasPrefix(line, "suffix:") {
		matcher = strings.TrimSpace(strings.TrimPrefix(line, "suffix:"))
	}
	if d := cleanDomain(matcher); d != "" && seen.addDomainQuery(d, kind, outbound) {
		return sink.OnDomainQuery(DomainQuery{Matcher: d, Kind: kind, Outbound: outbound})
	}
	return nil
}
