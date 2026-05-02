import { describe, it, expect } from 'vitest';
import { chainOutboundLabel, parseSnapshot } from './singboxConnections';
import type { ClashConnectionsRaw } from '$lib/types/singboxConnections';

describe('chainOutboundLabel', () => {
	it('returns "—" for empty chains', () => {
		expect(chainOutboundLabel([])).toBe('—');
	});
	it('translates DIRECT to a Russian label', () => {
		expect(chainOutboundLabel(['DIRECT'])).toBe('Прямое');
	});
	it('translates REJECT to a Russian label', () => {
		expect(chainOutboundLabel(['REJECT'])).toBe('Отклонено');
	});
	it('returns chains[0] for everything else', () => {
		expect(chainOutboundLabel(['vless-1', 'auto'])).toBe('vless-1');
	});
});

describe('parseSnapshot', () => {
	const baseRaw: ClashConnectionsRaw = {
		downloadTotal: 1234,
		uploadTotal: 567,
		connections: [
			{
				id: 'a',
				metadata: {
					network: 'tcp',
					type: 'Tun',
					sourceIP: '192.168.1.5',
					sourcePort: '53412',
					destinationIP: '142.250.74.110',
					destinationPort: '443',
					host: 'youtube.com',
				},
				upload: 100,
				download: 800,
				start: '2026-05-02T10:00:00Z',
				chains: ['vless-1'],
				rule: 'DOMAIN-SUFFIX',
				rulePayload: 'youtube.com',
			},
		],
	};

	it('enriches clientName from IP map (case-insensitive lookup)', () => {
		const clients = new Map([['192.168.1.5', 'iPhone']]);
		const snap = parseSnapshot(baseRaw, clients);
		expect(snap.connections[0].clientName).toBe('iPhone');
	});

	it('lowercases sourceIP for lookup', () => {
		const raw = structuredClone(baseRaw);
		raw.connections![0].metadata.sourceIP = 'FE80::1';
		const clients = new Map([['fe80::1', 'ipv6']]);
		const snap = parseSnapshot(raw, clients);
		expect(snap.connections[0].clientName).toBe('ipv6');
	});

	it('leaves clientName undefined when no match', () => {
		const snap = parseSnapshot(baseRaw, new Map());
		expect(snap.connections[0].clientName).toBeUndefined();
	});

	it('computes outboundLabel from chains[0]', () => {
		const snap = parseSnapshot(baseRaw, new Map());
		expect(snap.connections[0].outboundLabel).toBe('vless-1');
	});

	it('handles empty connections array', () => {
		const snap = parseSnapshot({ connections: [], downloadTotal: 0, uploadTotal: 0 }, new Map());
		expect(snap.connections).toEqual([]);
		expect(snap.connectionsTotal).toBe(0);
	});

	it('handles missing connections field', () => {
		const snap = parseSnapshot({}, new Map());
		expect(snap.connections).toEqual([]);
	});

	it('passes through totals', () => {
		const snap = parseSnapshot(baseRaw, new Map());
		expect(snap.downloadTotal).toBe(1234);
		expect(snap.uploadTotal).toBe(567);
		expect(snap.connectionsTotal).toBe(1);
	});
});
