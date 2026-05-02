// frontend/src/lib/utils/singboxConnections.ts

import type {
	ClashConnectionsRaw,
	Connection,
	ConnectionsSnapshot,
} from '$lib/types/singboxConnections';

const OUTBOUND_LABELS: Record<string, string> = {
	DIRECT: 'Прямое',
	REJECT: 'Отклонено',
};

export function chainOutboundLabel(chains: string[]): string {
	if (chains.length === 0) return '—';
	const first = chains[0];
	return OUTBOUND_LABELS[first] ?? first;
}

export function parseSnapshot(
	raw: ClashConnectionsRaw,
	clientsByIP: Map<string, string>,
): ConnectionsSnapshot {
	const rawConns = raw.connections ?? [];
	const connections: Connection[] = rawConns.map((c) => ({
		...c,
		clientName: clientsByIP.get(c.metadata.sourceIP.toLowerCase()),
		outboundLabel: chainOutboundLabel(c.chains),
	}));
	return {
		connections,
		downloadTotal: raw.downloadTotal ?? 0,
		uploadTotal: raw.uploadTotal ?? 0,
		connectionsTotal: connections.length,
	};
}
