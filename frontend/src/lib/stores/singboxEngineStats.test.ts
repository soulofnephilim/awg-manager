import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';
import { singboxTraffic } from './singbox';
import { singboxTrafficLive } from './singboxEngineStats';

function trafficMap(entries: Array<[string, { upload: number; download: number }]>) {
	return new Map(entries.map(([tag, t]) => [tag, { tag, ...t }]));
}

describe('singboxTrafficLive', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		singboxTraffic.set(new Map());
	});
	afterEach(() => {
		vi.useRealTimers();
	});

	it('первый снимок — объёмы есть, скорости нет', () => {
		singboxTraffic.set(trafficMap([['vpn', { upload: 100, download: 1000 }]]));
		const live = get(singboxTrafficLive);
		expect(live.totals.downloadBytes).toBe(1000);
		expect(live.totals.uploadBytes).toBe(100);
		expect(live.rate.hasRate).toBe(false);
	});

	it('второй снимок через 2с — скорость = дельта/время', () => {
		const unsub = singboxTrafficLive.subscribe(() => {});
		singboxTraffic.set(trafficMap([['vpn', { upload: 0, download: 0 }]]));
		vi.advanceTimersByTime(2000);
		singboxTraffic.set(trafficMap([['vpn', { upload: 200, download: 2000 }]]));
		const live = get(singboxTrafficLive);
		expect(live.rate.hasRate).toBe(true);
		expect(live.rate.downloadRate).toBe(1000);
		expect(live.rate.uploadRate).toBe(100);
		unsub();
	});

	it('сброс счётчиков (рестарт движка) не даёт отрицательную скорость', () => {
		const unsub = singboxTrafficLive.subscribe(() => {});
		singboxTraffic.set(trafficMap([['vpn', { upload: 500, download: 5000 }]]));
		vi.advanceTimersByTime(2000);
		singboxTraffic.set(trafficMap([['vpn', { upload: 10, download: 20 }]]));
		const live = get(singboxTrafficLive);
		expect(live.rate.hasRate).toBe(false);
		expect(live.rate.downloadRate).toBe(0);
		unsub();
	});
});
