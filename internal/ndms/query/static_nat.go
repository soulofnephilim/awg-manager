package query

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hoaxisr/awg-manager/internal/ndms/cache"
)

// StaticNATEntry is one row from /show/rc/ip/static.
type StaticNATEntry struct {
	Interface    string `json:"interface"`
	ToInterface  string `json:"to-interface"`
}

const staticNATTTL = 30 * time.Second

// StaticNATStore caches /show/rc/ip/static.
type StaticNATStore struct {
	*cache.ListStore[[]StaticNATEntry]
	getter Getter
}

func NewStaticNATStore(g Getter, log Logger) *StaticNATStore {
	s := &StaticNATStore{getter: g}
	s.ListStore = cache.NewListStore(staticNATTTL, log, "ip-static", s.fetch)
	return s
}

// ForInterface reports whether static NAT is configured for iface and the WAN target.
func (s *StaticNATStore) ForInterface(ctx context.Context, iface string) (bool, string, error) {
	entries, err := s.List(ctx)
	if err != nil {
		return false, "", err
	}
	for _, e := range entries {
		if e.Interface == iface {
			return true, e.ToInterface, nil
		}
	}
	return false, "", nil
}

func (s *StaticNATStore) fetch(ctx context.Context) ([]StaticNATEntry, error) {
	raw, err := s.getter.GetRaw(ctx, "/show/rc/ip/static")
	if err != nil {
		return nil, fmt.Errorf("fetch ip static: %w", err)
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var entries []StaticNATEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		var single StaticNATEntry
		if err2 := json.Unmarshal(raw, &single); err2 != nil {
			return nil, fmt.Errorf("decode ip static: %w", err)
		}
		if single.Interface != "" {
			return []StaticNATEntry{single}, nil
		}
		return nil, nil
	}
	return entries, nil
}
