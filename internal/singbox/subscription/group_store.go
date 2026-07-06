package subscription

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/hoaxisr/awg-manager/internal/storage"
)

// AggregateGroup — сводная группа: один selector/urltest outbound поверх
// членов НЕСКОЛЬКИХ подписок (#372). Группа не владеет member-outbound'ами
// (ими владеют подписки), она только ссылается на их теги; поэтому пересборка
// групп (stageGroups) обязана происходить в одном батче с любой мутацией
// подписок.
type AggregateGroup struct {
	ID         string `json:"id"`         // 12 rand bytes hex, как у подписок
	Label      string `json:"label"`      // user-facing
	Tag        string `json:"tag"`        // "agg-<id8>"
	InboundTag string `json:"inboundTag"` // "agg-<id8>-in"
	ListenPort uint16 `json:"listenPort"` // localhost-порт mixed inbound
	ProxyIndex int    `json:"proxyIndex"` // NDMS ProxyN, -1 когда нет
	// Mode — selector | urltest. Пустое значение трактуем как urltest:
	// основной кейс issue — «авто-выбор быстрейшего среди всех подписок».
	Mode    SubscriptionMode `json:"mode,omitempty"`
	URLTest *URLTestConfig   `json:"urlTest,omitempty"`
	// UseSubscriptionIDs — ID подписок-источников в пользовательском порядке
	// (определяет порядок членов в группе).
	UseSubscriptionIDs []string `json:"useSubscriptionIds"`
	FilterInclude      string   `json:"filterInclude,omitempty"`
	FilterExclude      string   `json:"filterExclude,omitempty"`
	Enabled            bool     `json:"enabled"`
}

// EffectiveMode — режим группы с back-compat шимом: пустая строка = urltest
// (в отличие от подписок, где дефолт selector — см. комментарий у Mode).
func (g AggregateGroup) EffectiveMode() SubscriptionMode {
	if g.Mode == "" {
		return ModeURLTest
	}
	return g.Mode
}

// EffectiveURLTest — URLTest группы с заполненными дефолтами.
func (g AggregateGroup) EffectiveURLTest() URLTestConfig {
	def := DefaultURLTestConfig()
	if g.URLTest == nil {
		return def
	}
	out := *g.URLTest
	if out.URL == "" {
		out.URL = def.URL
	}
	if out.IntervalSec <= 0 {
		out.IntervalSec = def.IntervalSec
	}
	if out.ToleranceMs < 0 {
		out.ToleranceMs = def.ToleranceMs
	}
	return out
}

// GroupCreateInput — вход Service.CreateGroup.
type GroupCreateInput struct {
	Label              string
	Mode               SubscriptionMode // "" = ModeURLTest
	URLTest            *URLTestConfig
	UseSubscriptionIDs []string
	FilterInclude      string
	FilterExclude      string
	Enabled            bool
}

// GroupUpdatePatch — частичный апдейт; nil-указатели = «оставить как есть».
type GroupUpdatePatch struct {
	Label              *string
	Mode               *SubscriptionMode
	URLTest            *URLTestConfig
	UseSubscriptionIDs *[]string
	FilterInclude      *string
	FilterExclude      *string
	Enabled            *bool
}

// GroupStore хранит сводные группы в отдельном JSON-файле рядом с
// subscription store (atomic-write на каждую мутацию, те же паттерны).
type GroupStore struct {
	path string
	mu   sync.RWMutex
	data map[string]*AggregateGroup
}

func NewGroupStore(path string) (*GroupStore, error) {
	s := &GroupStore{path: path, data: make(map[string]*AggregateGroup)}
	if err := s.load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return s, nil
}

func (s *GroupStore) load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	var list []*AggregateGroup
	if err := json.Unmarshal(b, &list); err != nil {
		return fmt.Errorf("subscription group store: parse %s: %w", s.path, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range list {
		s.data[item.ID] = item
	}
	return nil
}

func (s *GroupStore) saveLocked() error {
	list := make([]*AggregateGroup, 0, len(s.data))
	for _, item := range s.data {
		list = append(list, item)
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return storage.AtomicWrite(s.path, b)
}

func (s *GroupStore) Create(in GroupCreateInput) (*AggregateGroup, error) {
	id := newID()
	short := id[:8]
	mode := in.Mode
	if mode == "" {
		mode = ModeURLTest
	}
	var urlTest *URLTestConfig
	if mode == ModeURLTest {
		cfg := DefaultURLTestConfig()
		if in.URLTest != nil {
			if in.URLTest.URL != "" {
				cfg.URL = in.URLTest.URL
			}
			if in.URLTest.IntervalSec > 0 {
				cfg.IntervalSec = in.URLTest.IntervalSec
			}
			if in.URLTest.ToleranceMs >= 0 {
				cfg.ToleranceMs = in.URLTest.ToleranceMs
			}
		}
		urlTest = &cfg
	}
	useIDs := in.UseSubscriptionIDs
	if useIDs == nil {
		useIDs = []string{}
	}
	g := &AggregateGroup{
		ID:                 id,
		Label:              in.Label,
		Tag:                "agg-" + short,
		InboundTag:         "agg-" + short + "-in",
		ProxyIndex:         -1,
		Mode:               mode,
		URLTest:            urlTest,
		UseSubscriptionIDs: useIDs,
		FilterInclude:      in.FilterInclude,
		FilterExclude:      in.FilterExclude,
		Enabled:            in.Enabled,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[id] = g
	if err := s.saveLocked(); err != nil {
		delete(s.data, id)
		return nil, err
	}
	cp := *g
	return &cp, nil
}

func (s *GroupStore) Get(id string) (*AggregateGroup, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.data[id]
	if !ok {
		return nil, fmt.Errorf("subscription group %q not found", id)
	}
	cp := *g
	return &cp, nil
}

func (s *GroupStore) List() []AggregateGroup {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AggregateGroup, 0, len(s.data))
	for _, g := range s.data {
		out = append(out, *g)
	}
	return out
}

func (s *GroupStore) Update(id string, patch GroupUpdatePatch) (*AggregateGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.data[id]
	if !ok {
		return nil, fmt.Errorf("subscription group %q not found", id)
	}
	if patch.Label != nil {
		g.Label = *patch.Label
	}
	if patch.Mode != nil {
		g.Mode = *patch.Mode
		if g.Mode == ModeURLTest && g.URLTest == nil {
			cfg := DefaultURLTestConfig()
			g.URLTest = &cfg
		}
	}
	if patch.URLTest != nil {
		cp := *patch.URLTest
		g.URLTest = &cp
	}
	if patch.UseSubscriptionIDs != nil {
		ids := append([]string{}, (*patch.UseSubscriptionIDs)...)
		g.UseSubscriptionIDs = ids
	}
	if patch.FilterInclude != nil {
		g.FilterInclude = *patch.FilterInclude
	}
	if patch.FilterExclude != nil {
		g.FilterExclude = *patch.FilterExclude
	}
	if patch.Enabled != nil {
		g.Enabled = *patch.Enabled
	}
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	cp := *g
	return &cp, nil
}

func (s *GroupStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[id]; !ok {
		return fmt.Errorf("subscription group %q not found", id)
	}
	delete(s.data, id)
	return s.saveLocked()
}

// SetListenPort persists the allocated mixed-inbound port.
func (s *GroupStore) SetListenPort(id string, port uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.data[id]
	if !ok {
		return fmt.Errorf("subscription group %q not found", id)
	}
	g.ListenPort = port
	return s.saveLocked()
}

// SetProxyIndex persists the NDMS ProxyN index for this group.
func (s *GroupStore) SetProxyIndex(id string, idx int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.data[id]
	if !ok {
		return fmt.Errorf("subscription group %q not found", id)
	}
	g.ProxyIndex = idx
	return s.saveLocked()
}

// RemoveSubscriptionRef выкидывает subID из useSubscriptionIds всех групп
// (вызывается при удалении подписки; сами группы остаются).
func (s *GroupStore) RemoveSubscriptionRef(subID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for _, g := range s.data {
		kept := g.UseSubscriptionIDs[:0:0]
		for _, id := range g.UseSubscriptionIDs {
			if id == subID {
				changed = true
				continue
			}
			kept = append(kept, id)
		}
		if kept == nil {
			kept = []string{}
		}
		g.UseSubscriptionIDs = kept
	}
	if !changed {
		return nil
	}
	return s.saveLocked()
}
