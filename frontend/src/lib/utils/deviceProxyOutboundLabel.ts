// Приоритеты подписи outbound'а на карточках device-proxy (issue #465).
//
// Имя outbound'а — это НАСТРОЕННЫЙ выбор пользователя (selectedOutbound из
// конфига), а не живой selector.now: после выключения движка маршрутизации
// runtime-селектор мог деградировать (prune / graceful-fallback), и показывать
// его как «настройку» — враньё. Прецедент: about-device.ts (selectedOutbound
// первичен, activeTag — отдельная пометка «активный: X»).

import type { DeviceProxyRuntime } from '$lib/types';

export interface OutboundLabelInput {
	/** Настроенный outbound инстанса (instance.selectedOutbound). */
	selectedOutbound: string;
	/** Живое runtime-состояние; null/undefined — ещё не загружено. */
	runtime?: Pick<
		DeviceProxyRuntime,
		'alive' | 'activeTag' | 'defaultTag' | 'degradedOutbound' | 'fallbackTag'
	> | null;
}

/**
 * Имя outbound'а для бейджа: конфиг первичен, runtime.defaultTag — запасной
 * вариант (например, если конфиг ещё не догрузился), «—» — когда ничего нет.
 */
export function outboundName(input: OutboundLabelInput): string {
	return input.selectedOutbound || input.runtime?.defaultTag || '—';
}

/**
 * Живой selector.now для отдельной muted-пометки «сейчас: X». Показываем
 * ТОЛЬКО когда движок жив и активный член отличается от настроенного имени —
 * иначе пометка дублирует бейдж.
 */
export function outboundNowTag(input: OutboundLabelInput): string | null {
	const rt = input.runtime;
	if (!rt?.alive || !rt.activeTag) return null;
	return rt.activeTag !== outboundName(input) ? rt.activeTag : null;
}

/**
 * Текст бейджа деградации: настроенный выход отсутствует в merged-конфиге
 * (слот-источник выключен), трафик идёт через fallback. null — нет деградации.
 */
export function outboundDegradedText(input: OutboundLabelInput): string | null {
	const rt = input.runtime;
	if (!rt?.degradedOutbound) return null;
	const via = rt.fallbackTag || 'direct';
	return `выход недоступен — через ${via}`;
}
