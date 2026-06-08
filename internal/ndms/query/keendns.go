package query

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/hoaxisr/awg-manager/internal/ndms/cache"
)

const keenDNSTTL = 60 * time.Second

// KeenDNSInfo holds the router's KeenDNS domain registration, if any.
type KeenDNSInfo struct {
	Domain  string `json:"domain"`
	Enabled bool   `json:"enabled"`
}

// KeenDNSStore caches KeenDNS status from NDMS.
type KeenDNSStore struct {
	*cache.KeyedStore[string, *KeenDNSInfo]
	getter Getter
	log    Logger
}

func NewKeenDNSStore(g Getter, log Logger) *KeenDNSStore {
	s := &KeenDNSStore{getter: g, log: log}
	s.KeyedStore = cache.NewKeyedStore(keenDNSTTL, log, "keendns", s.fetch)
	return s
}

// Get returns the current KeenDNS registration. Missing/unconfigured → nil, nil.
func (s *KeenDNSStore) Get(ctx context.Context) (*KeenDNSInfo, error) {
	return s.KeyedStore.Get(ctx, "status")
}

func (s *KeenDNSStore) fetch(ctx context.Context, _ string) (*KeenDNSInfo, error) {
	for _, path := range []string{"/show/ndns", "/show/sc/ndns", "/show/ip/dns/domain"} {
		info, err := s.fetchFromPath(ctx, path)
		if err != nil {
			s.log.Warnf("keendns: %s: %v", path, err)
			continue
		}
		if info != nil && info.Domain != "" {
			return info, nil
		}
	}
	return nil, nil
}

func (s *KeenDNSStore) fetchFromPath(ctx context.Context, path string) (*KeenDNSInfo, error) {
	raw, err := s.getter.GetRaw(ctx, path)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("{}")) {
		return nil, nil
	}
	return parseKeenDNSJSON(trimmed), nil
}

func parseKeenDNSJSON(raw []byte) *KeenDNSInfo {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	domain := findKeenDNSDomain(v)
	if domain == "" {
		return nil
	}
	return &KeenDNSInfo{Domain: domain, Enabled: true}
}

func findKeenDNSDomain(v any) string {
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if looksLikeKeenDNSDomain(s) {
			return s
		}
	case map[string]any:
		for _, key := range []string{"domain", "name", "hostname", "fqdn", "address"} {
			if raw, ok := t[key]; ok {
				if d := findKeenDNSDomain(raw); d != "" {
					return d
				}
			}
		}
		for _, child := range t {
			if d := findKeenDNSDomain(child); d != "" {
				return d
			}
		}
	case []any:
		for _, item := range t {
			if d := findKeenDNSDomain(item); d != "" {
				return d
			}
		}
	}
	return ""
}

func looksLikeKeenDNSDomain(s string) bool {
	if s == "" || strings.Contains(s, " ") {
		return false
	}
	lower := strings.ToLower(s)
	return strings.HasSuffix(lower, ".keenetic.pro") ||
		strings.HasSuffix(lower, ".keenetic.name") ||
		strings.HasSuffix(lower, ".keenetic.link") ||
		strings.HasSuffix(lower, ".netcraze.pro") ||
		strings.HasSuffix(lower, ".netcraze.link")
}
