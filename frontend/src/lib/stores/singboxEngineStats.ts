// Живой агрегатный трафик движка sing-box (скорость + объём за сессию) для
// TProxy-вкладки. Дельта кумулятивных сумм между двумя снимками SSE
// singbox:traffic — та же техника, что TrafficPanel на FakeIP «Обзор».
//
// Память сюда НЕ входит: singbox:memory приходит отдельным SSE-событием почти
// одновременно с traffic, и общий derived стрелял бы дважды за тик —
// computeRate с защитой dt ≤ 0.5s сбрасывал бы скорость в «—» на каждом
// втором пересчёте. Память читается напрямую из singboxMemory.
import { derived } from 'svelte/store';
import { singboxTraffic } from './singbox';
import {
	aggregateTotals,
	computeRate,
	type RateSnapshot,
	type TrafficRate,
	type TrafficTotals,
} from '$lib/utils/singboxTrafficRate';

export interface SingboxTrafficLive {
	totals: TrafficTotals;
	rate: TrafficRate;
}

let prev: RateSnapshot | null = null;

export const singboxTrafficLive = derived(singboxTraffic, (map): SingboxTrafficLive => {
	const totals = aggregateTotals(map);
	const next: RateSnapshot = {
		timestamp: Date.now(),
		downloadBytes: totals.downloadBytes,
		uploadBytes: totals.uploadBytes,
	};
	const rate = computeRate(prev, next);
	prev = next;
	return { totals, rate };
});
