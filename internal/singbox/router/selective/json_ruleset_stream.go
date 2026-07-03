package selective

import (
	"encoding/json"
	"io"
	"os"
	"strings"
)

// streamRuleSetJSONFile walks rules in a rule-set source JSON without loading
// the entire file when domain_suffix arrays are large.
func streamRuleSetJSONFile(path string, outbound string, seen *deduplicator, sink *CollectSink) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, _ := keyTok.(string)
		if key != "rules" {
			if err := skipJSONValue(dec); err != nil {
				return err
			}
			continue
		}
		arrTok, err := dec.Token()
		if err != nil {
			return err
		}
		if d, ok := arrTok.(json.Delim); !ok || d != '[' {
			continue
		}
		for dec.More() {
			var rule map[string]json.RawMessage
			if err := dec.Decode(&rule); err != nil {
				return err
			}
			if err := streamExtractFromRuleMap(rule, outbound, seen, sink); err != nil {
				return err
			}
		}
		if _, err := dec.Token(); err != nil { // ]
			return err
		}
	}
	return nil
}

func streamExtractFromRuleMap(rule map[string]json.RawMessage, outbound string, seen *deduplicator, sink *CollectSink) error {
	if raw, ok := rule["ip_cidr"]; ok {
		if err := streamStringArray(raw, func(s string) error {
			if c := normalizeCIDR(s); c != "" {
				return sink.OnStaticCIDR(c)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	for _, key := range []string{"domain_suffix", "domain"} {
		raw, ok := rule[key]
		if !ok {
			continue
		}
		kind := KindDomain
		if key == "domain_suffix" {
			kind = KindDomainSuffix
		}
		if err := streamStringArray(raw, func(s string) error {
			if d := cleanDomain(s); d != "" && seen.addDomainQuery(d, kind, outbound) {
				return sink.OnDomainQuery(DomainQuery{Matcher: d, Kind: kind, Outbound: outbound})
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// streamExtractFromRuleValueMap mirrors streamExtractFromRuleMap for rules
// already decoded as map[string]interface{} (in-memory rule-sets restored by
// the router materializer), reading the values in place instead of paying a
// JSON marshal→unmarshal round-trip per rule.
func streamExtractFromRuleValueMap(rule map[string]interface{}, outbound string, seen *deduplicator, sink *CollectSink) error {
	if v, ok := rule["ip_cidr"]; ok {
		if err := forEachRuleString(v, func(s string) error {
			if c := normalizeCIDR(s); c != "" {
				return sink.OnStaticCIDR(c)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	for _, key := range []string{"domain_suffix", "domain"} {
		v, ok := rule[key]
		if !ok {
			continue
		}
		kind := KindDomain
		if key == "domain_suffix" {
			kind = KindDomainSuffix
		}
		if err := forEachRuleString(v, func(s string) error {
			if d := cleanDomain(s); d != "" && seen.addDomainQuery(d, kind, outbound) {
				return sink.OnDomainQuery(DomainQuery{Matcher: d, Kind: kind, Outbound: outbound})
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// forEachRuleString invokes fn for a scalar string or a string array value —
// both shapes appear in sing-box rule-set source rules. JSON-decoded maps
// carry []interface{}; programmatically built refs may carry []string.
func forEachRuleString(v interface{}, fn func(string) error) error {
	switch t := v.(type) {
	case string:
		return fn(t)
	case []interface{}:
		for _, e := range t {
			if s, ok := e.(string); ok {
				if err := fn(s); err != nil {
					return err
				}
			}
		}
	case []string:
		for _, s := range t {
			if err := fn(s); err != nil {
				return err
			}
		}
	}
	return nil
}

func streamStringArray(raw json.RawMessage, fn func(string) error) error {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return err
		}
		return fn(s)
	}
	if raw[0] != '[' {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	if tok, err := dec.Token(); err != nil {
		return err
	} else if d, ok := tok.(json.Delim); !ok || d != '[' {
		return nil
	}
	for dec.More() {
		var s string
		if err := dec.Decode(&s); err != nil {
			return err
		}
		if err := fn(s); err != nil {
			return err
		}
	}
	return nil
}

func skipJSONValue(dec *json.Decoder) error {
	var skip json.RawMessage
	return dec.Decode(&skip)
}

// streamSRSJSONFile reads decompiled SRS JSON from a file path line-by-line rule.
func streamSRSJSONFile(path string, outbound string, seen *deduplicator, sink *CollectSink) error {
	return streamRuleSetJSONFile(path, outbound, seen, sink)
}

// noop closer for io usage
var _ io.Closer = (*os.File)(nil)
