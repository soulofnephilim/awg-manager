package command

import (
	"context"
	"fmt"
	"strings"
)

// NDMS access-list примитивы — единственная точка ACL-мутаций (SET только
// через parse: структурные формы NDMS отвергает). Потребители: fakeip-tun
// (permit-all на OpkgTun, композиции ниже) и managed-серверы (гранулярные
// permit'ы peer→сегмент). Все через postMutationChecked: parse-ответы NDMS
// кладут ошибки во вложенный status[] («a duplicate was found», «cannot
// enable auto-deletion for unreferenced lists», «argument parse error» —
// stand-verified 2026-07-16), который транспортный уровень не видит.

// ACLPermitIP добавляет permit-правило (первый permit неявно создаёт список).
// Повторный идентичный permit NDMS отклоняет «a duplicate was found» БЕЗ
// дублирования правила — вызывающие, которым нужен идемпотентный re-assert,
// матчат IsACLDuplicate.
func (c *InterfaceCommands) ACLPermitIP(ctx context.Context, acl, srcSub, srcMask, dstSub, dstMask string) error {
	return postMutationChecked(ctx, c.poster, c.save,
		map[string]any{"parse": fmt.Sprintf("access-list %s permit ip %s %s %s %s", acl, srcSub, srcMask, dstSub, dstMask)},
		"acl permit "+acl,
		c.queries.RunningConfig.InvalidateAll,
	)
}

// ACLRemove удаляет список целиком (`no access-list`). Идемпотентно на
// уровне вызывающих: несуществующий список — ошибка, teardown-пути её логируют.
func (c *InterfaceCommands) ACLRemove(ctx context.Context, acl string) error {
	return postMutationChecked(ctx, c.poster, c.save,
		map[string]any{"parse": "no access-list " + acl},
		"acl remove "+acl,
		c.queries.RunningConfig.InvalidateAll,
	)
}

// ACLBind привязывает список `in` к интерфейсу. Повторная привязка
// идемпотентна (status message, stand-verified).
func (c *InterfaceCommands) ACLBind(ctx context.Context, iface, acl string) error {
	return postMutationChecked(ctx, c.poster, c.save,
		map[string]any{"parse": fmt.Sprintf("interface %s ip access-group %s in", iface, acl)},
		"acl bind "+acl,
		func() { c.queries.Interfaces.Invalidate(iface) },
		c.queries.RunningConfig.InvalidateAll,
	)
}

// ACLUnbind снимает привязку списка с интерфейса.
func (c *InterfaceCommands) ACLUnbind(ctx context.Context, iface, acl string) error {
	return postMutationChecked(ctx, c.poster, c.save,
		map[string]any{"parse": fmt.Sprintf("no interface %s ip access-group %s in", iface, acl)},
		"acl unbind "+acl,
		func() { c.queries.Interfaces.Invalidate(iface) },
		c.queries.RunningConfig.InvalidateAll,
	)
}

// ACLAutoDelete включает каскадное удаление списка вместе с последним
// ссылающимся интерфейсом. Работает ТОЛЬКО на привязанном списке («cannot
// enable auto-deletion for unreferenced lists») — вызывать после ACLBind.
// Повторное включение идемпотентно (stand-verified).
func (c *InterfaceCommands) ACLAutoDelete(ctx context.Context, acl string) error {
	return postMutationChecked(ctx, c.poster, c.save,
		map[string]any{"parse": fmt.Sprintf("access-list %s auto-delete", acl)},
		"acl auto-delete "+acl,
		c.queries.RunningConfig.InvalidateAll,
	)
}

// IsACLDuplicate распознаёт NDMS-отказ на повторный идентичный permit —
// безвредный случай для идемпотентного re-assert. Матчится ПОЛНАЯ фраза
// NDMS («a duplicate was found for the rule being set», stand-verified),
// а не слово «duplicate» — чтобы смешанный ответ с реальной ошибкой,
// случайно содержащей это слово, не был проглочен (ревью).
func IsACLDuplicate(err error) bool {
	return err != nil && strings.Contains(err.Error(), "a duplicate was found")
}

// SetPermitAllACL создаёт permit-all access-list `_WEBADMIN_<name>` (конвенция
// веб-морды Keenetic — UI показывает его как разрешение доступа к
// интерфейсу), привязывает `in` и включает auto-delete. Идемпотентен: дубль
// permit толерируется, повторные bind/auto-delete идемпотентны в NDMS.
func (c *InterfaceCommands) SetPermitAllACL(ctx context.Context, name string) error {
	acl := "_WEBADMIN_" + name
	if err := c.ACLPermitIP(ctx, acl, "0.0.0.0", "0.0.0.0", "0.0.0.0", "0.0.0.0"); err != nil && !IsACLDuplicate(err) {
		return err
	}
	if err := c.ACLBind(ctx, name, acl); err != nil {
		return err
	}
	return c.ACLAutoDelete(ctx, acl)
}

// RemovePermitAllACL снимает привязку и удаляет `_WEBADMIN_<name>`.
// Best-effort по замыслу: интерфейс может быть уже удалён, а auto-delete —
// уже каскадировать список; ошибки возвращаются вызывающему для лога,
// teardown они не фатальны.
func (c *InterfaceCommands) RemovePermitAllACL(ctx context.Context, name string) error {
	acl := "_WEBADMIN_" + name
	unbindErr := c.ACLUnbind(ctx, name, acl)
	removeErr := c.ACLRemove(ctx, acl)
	if unbindErr != nil {
		return unbindErr
	}
	return removeErr
}
