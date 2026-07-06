import { describe, it, expect } from 'vitest';
import { resolveGroupPreview } from './subscriptionGroupPreview';
import type { Subscription, SubscriptionMember } from '$lib/types';

function member(tag: string, label: string): SubscriptionMember {
	return { tag, label, protocol: 'vless', server: `${tag}.example`, port: 443 };
}

function sub(id: string, enabled: boolean, members: SubscriptionMember[]): Subscription {
	return {
		id,
		label: id,
		url: `https://example.com/${id}`,
		isInline: false,
		headers: [],
		refreshHours: 0,
		lastFetched: '',
		selectorTag: `sub-${id}`,
		inboundTag: `sub-${id}-in`,
		listenPort: 11000,
		proxyIndex: -1,
		memberTags: members.map((m) => m.tag),
		members,
		orphanTags: [],
		activeMember: members[0]?.tag ?? '',
		enabled,
		mode: 'selector',
	};
}

const subs: Subscription[] = [
	sub('a', true, [member('a1', '🇩🇪 DE-1'), member('a2', '🇷🇺 RU-1')]),
	sub('b', true, [member('b1', 'NL-1'), member('b2', 'NL-2')]),
	sub('off', false, [member('o1', 'DE-off')]),
];

describe('resolveGroupPreview', () => {
	it('объединяет подписки в порядке useSubscriptionIds', () => {
		const res = resolveGroupPreview(subs, ['b', 'a'], '', '');
		expect(res.count).toBe(4);
		expect(res.labels).toEqual(['NL-1', 'NL-2', '🇩🇪 DE-1', '🇷🇺 RU-1']);
	});

	it('пропускает выключенные и несуществующие подписки', () => {
		const res = resolveGroupPreview(subs, ['off', 'missing', 'a'], '', '');
		expect(res.labels).toEqual(['🇩🇪 DE-1', '🇷🇺 RU-1']);
	});

	it('схлопывает дубликаты id подписок', () => {
		const res = resolveGroupPreview(subs, ['a', 'a'], '', '');
		expect(res.count).toBe(2);
	});

	it('применяет include и exclude вместе', () => {
		const res = resolveGroupPreview(subs, ['a', 'b'], 'DE|NL', 'NL-2');
		expect(res.labels).toEqual(['🇩🇪 DE-1', 'NL-1']);
	});

	it('фильтрует по эмодзи-флагам', () => {
		const res = resolveGroupPreview(subs, ['a'], '', '🇷🇺');
		expect(res.labels).toEqual(['🇩🇪 DE-1']);
	});

	it('флагует некомпилируемый шаблон и не ограничивает по нему', () => {
		const res = resolveGroupPreview(subs, ['b'], '(broken', '');
		expect(res.invalidInclude).toBe(true);
		expect(res.count).toBe(2); // битый include игнорируется в превью
	});

	it('пустое имя сервера подставляет server:port', () => {
		const withEmpty = [sub('c', true, [member('c1', '')])];
		const res = resolveGroupPreview(withEmpty, ['c'], '', '');
		expect(res.labels).toEqual(['c1.example:443']);
	});
});
