import type {
	ExternalTunnel,
	Subscription,
	SubscriptionMember,
	SingboxTunnel,
	SystemTunnel,
	TunnelListItem,
} from '$lib/types';
import { compareString } from '$lib/utils/tunnelTableSort';

export type SubscriptionActiveCard = {
	subscription: Subscription;
	activeMember: SubscriptionMember;
};

export type TunnelDashboardFlatItem =
	| { kind: 'awg-managed'; key: string; name: string; tunnel: TunnelListItem; index: number }
	| { kind: 'awg-system'; key: string; name: string; tunnel: SystemTunnel }
	| { kind: 'awg-external'; key: string; name: string; tunnel: ExternalTunnel }
	| { kind: 'singbox'; key: string; name: string; tunnel: SingboxTunnel; index: number }
	| { kind: 'sub-active'; key: string; name: string; card: SubscriptionActiveCard; index: number }
	| { kind: 'sub-stopped'; key: string; name: string; subscription: Subscription };

/** Flat dashboard interleave order (alphabetical within each group). */
const FLAT_DASHBOARD_KIND_ORDER: Record<TunnelDashboardFlatItem['kind'], number> = {
	'awg-managed': 0,
	'awg-system': 1,
	'awg-external': 2,
	singbox: 3,
	'sub-active': 4,
	'sub-stopped': 5,
};

function compareFlatDashboardItems(a: TunnelDashboardFlatItem, b: TunnelDashboardFlatItem): number {
	const byKind = FLAT_DASHBOARD_KIND_ORDER[a.kind] - FLAT_DASHBOARD_KIND_ORDER[b.kind];
	if (byKind !== 0) return byKind;
	return compareString(a.name, b.name);
}

export function buildFlatDashboardItems(input: {
	awg: TunnelListItem[];
	system: SystemTunnel[];
	external: ExternalTunnel[];
	singbox: SingboxTunnel[];
	subscriptionsActive: SubscriptionActiveCard[];
	subscriptionsStopped: Subscription[];
}): TunnelDashboardFlatItem[] {
	const items: TunnelDashboardFlatItem[] = [];

	input.awg.forEach((tunnel, index) => {
		items.push({
			kind: 'awg-managed',
			key: `awg:${tunnel.id}`,
			name: tunnel.name,
			tunnel,
			index,
		});
	});

	for (const tunnel of input.system) {
		items.push({
			kind: 'awg-system',
			key: `system:${tunnel.id}`,
			name: tunnel.description || tunnel.interfaceName || tunnel.id,
			tunnel,
		});
	}

	for (const tunnel of input.external) {
		items.push({
			kind: 'awg-external',
			key: `external:${tunnel.interfaceName}`,
			name: tunnel.interfaceName,
			tunnel,
		});
	}

	input.singbox.forEach((tunnel, index) => {
		items.push({
			kind: 'singbox',
			key: `singbox:${tunnel.tag}`,
			name: tunnel.tag,
			tunnel,
			index,
		});
	});

	input.subscriptionsActive.forEach((card, index) => {
		items.push({
			kind: 'sub-active',
			key: `sub-active:${card.subscription.id}`,
			name: card.subscription.label,
			card,
			index,
		});
	});

	for (const subscription of input.subscriptionsStopped) {
		items.push({
			kind: 'sub-stopped',
			key: `sub-stopped:${subscription.id}`,
			name: subscription.label,
			subscription,
		});
	}

	return items.sort(compareFlatDashboardItems);
}
