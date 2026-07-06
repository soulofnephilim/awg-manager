package subscription

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrGroupsDisabled возвращается Group-CRUD, когда GroupStore не подключён
// (SetGroupStore не вызывался — тесты / legacy bootstrap).
var ErrGroupsDisabled = errors.New("subscription: aggregate groups are not configured")

// ErrGroupSubscriptionNotFound возвращается Create/UpdateGroup, когда
// useSubscriptionIds ссылается на несуществующую подписку.
var ErrGroupSubscriptionNotFound = errors.New("subscription: группа ссылается на несуществующую подписку")

// SetGroupStore подключает хранилище сводных групп. Вызывается один раз
// при bootstrap; nil-store оставляет функциональность выключенной.
func (s *Service) SetGroupStore(gs *GroupStore) { s.groups = gs }

// ListGroups возвращает все сводные группы.
func (s *Service) ListGroups() []AggregateGroup {
	if s.groups == nil {
		return nil
	}
	return s.groups.List()
}

// GetGroup возвращает одну сводную группу.
func (s *Service) GetGroup(id string) (*AggregateGroup, error) {
	if s.groups == nil {
		return nil, ErrGroupsDisabled
	}
	return s.groups.Get(id)
}

// ResolveGroupMembers собирает членов группы в детерминированном порядке:
// useSubscriptionIds в сохранённом порядке → для каждой существующей и
// включённой подписки её Members в сохранённом порядке → тег попадает в
// группу, когда фильтр группы пропускает имя сервера. Повторные ID подписок
// схлопываются (member-теги в группе не дублируются).
func (s *Service) ResolveGroupMembers(g AggregateGroup) ([]MemberInfo, error) {
	flt, err := CompileMemberFilter(g.FilterInclude, g.FilterExclude)
	if err != nil {
		return nil, fmt.Errorf("subscription group: %w", err)
	}
	out := []MemberInfo{}
	seenSub := make(map[string]bool, len(g.UseSubscriptionIDs))
	for _, subID := range g.UseSubscriptionIDs {
		if seenSub[subID] {
			continue
		}
		seenSub[subID] = true
		sub, err := s.store.Get(subID)
		if err != nil || !sub.Enabled {
			continue // удалённая или выключенная подписка не участвует
		}
		for _, m := range sub.Members {
			if flt.Allows(m.Label) {
				out = append(out, m)
			}
		}
	}
	return out, nil
}

// stageGroups пересобирает outbound/inbound/route каждой сводной группы в
// текущем НЕзакоммиченном батче мутатора. Reload внутри НЕ вызывается —
// вызывающая сторона коммитит всё одним flush (один SIGHUP на операцию,
// группы всегда консистентны с подписками). overrides позволяет refresh-пути
// подставить свежую (ещё не записанную в store) членскую базу подписки.
func (s *Service) stageGroups(overrides map[string][]MemberInfo) {
	if s.groups == nil {
		return
	}
	for _, g := range s.groups.List() {
		// Старый group outbound снимаем всегда (идемпотентно): при пустом
		// разрешённом наборе он не должен остаться висеть в конфиге.
		s.mutator.RemoveOutbound(g.Tag)
		if !g.Enabled {
			// Выключенная группа — полный teardown её сущностей в слоте.
			s.mutator.RemoveRouteRule(g.InboundTag, g.Tag)
			s.mutator.RemoveInbound(g.InboundTag)
			continue
		}
		tags, err := s.resolveGroupTags(g, overrides)
		if err != nil {
			// Битый фильтр из руками отредактированного файла: не паникуем,
			// снимаем сущности группы и пишем warning.
			s.logWarn("subscription-group", g.ID, "skip group (bad filter): "+err.Error())
			s.mutator.RemoveRouteRule(g.InboundTag, g.Tag)
			s.mutator.RemoveInbound(g.InboundTag)
			continue
		}
		if len(tags) == 0 {
			// Пустой selector/urltest sing-box отвергает — группу оставляем
			// в store (UI покажет «0 серверов»), но outbound не эмитим и
			// route-правило снимаем. Inbound сохраняем: он держит занятым
			// listen_port (иначе AllocListenPort мог бы выдать его другому)
			// и структурно валиден без правила.
			s.mutator.RemoveRouteRule(g.InboundTag, g.Tag)
			if g.ListenPort != 0 {
				s.mutator.AddInbound(g.InboundTag, BuildMixedInbound(g.InboundTag, g.ListenPort))
			}
			continue
		}
		if err := s.mutator.AddOutbound(g.Tag, BuildAggregateGroupOutbound(g, tags)); err != nil {
			s.logWarn("subscription-group", g.ID, "stage outbound failed: "+err.Error())
			continue
		}
		if g.ListenPort != 0 {
			s.mutator.AddInbound(g.InboundTag, BuildMixedInbound(g.InboundTag, g.ListenPort))
			s.mutator.AddRouteRule(BuildRouteRule(g.InboundTag, g.Tag))
		}
	}
}

// resolveGroupTags — теговая проекция ResolveGroupMembers с поддержкой
// overrides (subID → свежие члены, ещё не записанные в store).
func (s *Service) resolveGroupTags(g AggregateGroup, overrides map[string][]MemberInfo) ([]string, error) {
	flt, err := CompileMemberFilter(g.FilterInclude, g.FilterExclude)
	if err != nil {
		return nil, err
	}
	var tags []string
	seenSub := make(map[string]bool, len(g.UseSubscriptionIDs))
	for _, subID := range g.UseSubscriptionIDs {
		if seenSub[subID] {
			continue
		}
		seenSub[subID] = true
		sub, err := s.store.Get(subID)
		if err != nil || !sub.Enabled {
			continue // удалённая или выключенная подписка не участвует
		}
		members := sub.Members
		if ov, ok := overrides[subID]; ok {
			members = ov // свежий состав из идущего refresh (ещё не в store)
		}
		for _, m := range members {
			if flt.Allows(m.Label) {
				tags = append(tags, m.Tag)
			}
		}
	}
	return tags, nil
}

// reloadWithGroups — единая точка коммита для всех мутаций подписок:
// пересобирает сводные группы в том же батче и делает один Reload.
func (s *Service) reloadWithGroups(ctx context.Context) error {
	s.stageGroups(nil)
	return s.mutator.Reload(ctx)
}

// validateGroupInput — общая валидация Create/UpdateGroup.
func (s *Service) validateGroupSubs(ids []string) error {
	for _, id := range ids {
		if _, err := s.store.Get(id); err != nil {
			return fmt.Errorf("%w: %s", ErrGroupSubscriptionNotFound, id)
		}
	}
	return nil
}

// CreateGroup создаёт сводную группу: валидация, alloc listen-port, alloc
// ProxyN (когда глобальный тумблер включён), материализация + один Reload.
// Зеркалит Service.Create по rollback-семантике: при неудаче частично
// созданные сущности снимаются и строка удаляется из store.
func (s *Service) CreateGroup(ctx context.Context, in GroupCreateInput) (*AggregateGroup, error) {
	if s.groups == nil {
		return nil, ErrGroupsDisabled
	}
	if strings.TrimSpace(in.Label) == "" {
		return nil, errors.New("subscription: название группы не может быть пустым")
	}
	if _, err := CompileMemberFilter(in.FilterInclude, in.FilterExclude); err != nil {
		return nil, fmt.Errorf("subscription: %w", err)
	}
	if err := s.validateGroupSubs(in.UseSubscriptionIDs); err != nil {
		return nil, err
	}
	// Сериализуем с Create подписок: allocation сканирует без резервирования
	// (та же семантика, что и у подписок — issue #287).
	s.createMu.Lock()
	defer s.createMu.Unlock()
	s.groupMu.Lock()
	defer s.groupMu.Unlock()

	g, err := s.groups.Create(in)
	if err != nil {
		return nil, err
	}
	port, err := s.mutator.AllocListenPort()
	if err != nil {
		s.groups.Delete(g.ID)
		return nil, fmt.Errorf("subscription group: alloc listen port: %w", err)
	}
	if err := s.groups.SetListenPort(g.ID, port); err != nil {
		s.groups.Delete(g.ID)
		return nil, err
	}
	proxyIdx := -1
	if s.proxyEnabled() {
		idx, err := s.mutator.AllocProxyIndex(ctx)
		if err != nil {
			s.groups.Delete(g.ID)
			return nil, fmt.Errorf("subscription group: alloc proxy index: %w", err)
		}
		if err := s.groups.SetProxyIndex(g.ID, idx); err != nil {
			s.groups.Delete(g.ID)
			return nil, err
		}
		proxyIdx = idx
		if err := s.mutator.EnsureProxy(ctx, idx, int(port), g.Label); err != nil {
			_ = s.mutator.RemoveProxy(ctx, idx)
			s.groups.Delete(g.ID)
			return nil, fmt.Errorf("subscription group: register NDMS proxy: %w", err)
		}
	}

	if err := s.reloadWithGroups(ctx); err != nil {
		s.mutator.Rollback()
		if proxyIdx >= 0 {
			_ = s.mutator.RemoveProxy(ctx, proxyIdx)
		}
		s.groups.Delete(g.ID)
		return nil, fmt.Errorf("subscription group: materialize: %w", err)
	}

	final, err := s.groups.Get(g.ID)
	if err != nil {
		return nil, err
	}
	s.logInfo("subscription-group-create", g.ID, fmt.Sprintf("created mode=%s subs=%d listen_port=%d proxy_index=%d", final.EffectiveMode(), len(final.UseSubscriptionIDs), final.ListenPort, final.ProxyIndex))
	return final, nil
}

// UpdateGroup применяет частичный патч и пере-материализует группу
// (stage + один Reload). Смена label пробрасывается в описание NDMS Proxy.
func (s *Service) UpdateGroup(ctx context.Context, id string, patch GroupUpdatePatch) (*AggregateGroup, error) {
	if s.groups == nil {
		return nil, ErrGroupsDisabled
	}
	s.groupMu.Lock()
	defer s.groupMu.Unlock()

	current, err := s.groups.Get(id)
	if err != nil {
		return nil, err
	}
	if patch.Label != nil && strings.TrimSpace(*patch.Label) == "" {
		return nil, errors.New("subscription: название группы не может быть пустым")
	}
	if patch.FilterInclude != nil || patch.FilterExclude != nil {
		newInclude, newExclude := current.FilterInclude, current.FilterExclude
		if patch.FilterInclude != nil {
			newInclude = *patch.FilterInclude
		}
		if patch.FilterExclude != nil {
			newExclude = *patch.FilterExclude
		}
		if _, err := CompileMemberFilter(newInclude, newExclude); err != nil {
			return nil, fmt.Errorf("subscription: %w", err)
		}
	}
	if patch.UseSubscriptionIDs != nil {
		if err := s.validateGroupSubs(*patch.UseSubscriptionIDs); err != nil {
			return nil, err
		}
	}
	g, err := s.groups.Update(id, patch)
	if err != nil {
		return nil, err
	}
	if err := s.reloadWithGroups(ctx); err != nil {
		return g, fmt.Errorf("subscription group: reload: %w", err)
	}
	if patch.Label != nil && s.proxyEnabled() && g.ProxyIndex >= 0 {
		// EnsureProxy идемпотентен — обновляет описание ProxyN «на месте».
		if err := s.mutator.EnsureProxy(ctx, g.ProxyIndex, int(g.ListenPort), g.Label); err != nil {
			return g, fmt.Errorf("subscription group: sync proxy description: %w", err)
		}
	}
	s.logInfo("subscription-group-update", id, "updated")
	return g, nil
}

// DeleteGroup сносит группу целиком: outbound, route-правило, inbound,
// NDMS ProxyN и строку в store. Ошибки мутатора не блокируют удаление
// строки (симметрично Service.Delete для подписок).
func (s *Service) DeleteGroup(ctx context.Context, id string) error {
	if s.groups == nil {
		return ErrGroupsDisabled
	}
	s.groupMu.Lock()
	defer s.groupMu.Unlock()

	g, err := s.groups.Get(id)
	if err != nil {
		return err
	}
	s.mutator.RemoveRouteRule(g.InboundTag, g.Tag)
	s.mutator.RemoveInbound(g.InboundTag)
	s.mutator.RemoveOutbound(g.Tag)
	if g.ProxyIndex >= 0 {
		if err := s.mutator.RemoveProxy(ctx, g.ProxyIndex); err != nil {
			s.logWarn("subscription-group-delete", id, "remove proxy failed: "+err.Error())
		}
	}
	if err := s.groups.Delete(id); err != nil {
		return err
	}
	if err := s.reloadWithGroups(ctx); err != nil {
		return fmt.Errorf("subscription group: delete reload: %w", err)
	}
	s.logInfo("subscription-group-delete", id, "deleted")
	return nil
}
