// Группировка и подписи для зеркала inbound'ов merged-конфига sing-box
// (GET /api/singbox/inbounds). Чистые функции — используются общим
// компонентом InboundsMirror (tproxy ExpertPanel + fakeip InboundsTab)
// и покрыты vitest.

import type { SingboxInboundEntry, SingboxInboundSource } from '$lib/types';

/** Русские заголовки групп по источнику. */
export const INBOUND_GROUP_TITLES: Record<SingboxInboundSource, string> = {
	engine: 'Движок',
	deviceproxy: 'Прокси устройств',
	subscription: 'Подписки',
	group: 'Сводные группы',
	tunnel: 'Туннели',
	qos: 'QoS',
	other: 'Прочее',
};

/** Порядок групп в UI: движок первым, «прочее» последним. */
export const INBOUND_GROUP_ORDER: SingboxInboundSource[] = [
	'engine',
	'deviceproxy',
	'subscription',
	'group',
	'tunnel',
	'qos',
	'other',
];

export interface InboundGroup {
	source: SingboxInboundSource;
	title: string;
	entries: SingboxInboundEntry[];
}

/**
 * Группирует записи по источнику в фиксированном порядке INBOUND_GROUP_ORDER;
 * пустые группы опускаются, порядок записей внутри группы сохраняется.
 * Неизвестный source (расширение бэкенда) попадает в «Прочее».
 */
export function groupInbounds(entries: SingboxInboundEntry[]): InboundGroup[] {
	const bySource = new Map<SingboxInboundSource, SingboxInboundEntry[]>();
	for (const e of entries) {
		const source: SingboxInboundSource = INBOUND_GROUP_ORDER.includes(e.source)
			? e.source
			: 'other';
		const list = bySource.get(source);
		if (list) list.push(e);
		else bySource.set(source, [e]);
	}
	return INBOUND_GROUP_ORDER.filter((s) => bySource.has(s)).map((s) => ({
		source: s,
		title: INBOUND_GROUP_TITLES[s],
		entries: bySource.get(s) ?? [],
	}));
}

/** Подпись listen-адреса: "127.0.0.1:1080"; у tun порта/адреса нет — "—". */
export function inboundListenLabel(e: SingboxInboundEntry): string {
	if (!e.listen && !e.listenPort) return '—';
	return `${e.listen || '0.0.0.0'}:${e.listenPort}`;
}

/** Короткий бейдж для idle-записи (резерв порта). */
export function idleBadgeLabel(e: SingboxInboundEntry): string {
	if (!e.idle) return '';
	return e.idleReason === 'entity_disabled'
		? 'резерв порта — объект отключён'
		: 'резерв порта — NDMS-прокси выключен';
}

/** Развёрнутое пояснение (title-tooltip): почему inbound сохранён в конфиге. */
export function idleTitle(e: SingboxInboundEntry): string {
	if (!e.idle) return '';
	const cause =
		e.idleReason === 'entity_disabled'
			? 'его владелец (подписка/группа) отключён'
			: 'тумблер «Создавать NDMS-прокси» выключен и порт никто не питает';
	return `Inbound сохранён в конфиге, хотя ${cause}: порт остаётся зарезервированным, чтобы при включении не менялись номера портов.`;
}
