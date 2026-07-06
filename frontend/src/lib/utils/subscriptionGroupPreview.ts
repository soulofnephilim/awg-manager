import type { Subscription } from '$lib/types';

/**
 * Результат клиентского превью состава сводной группы.
 * ВНИМАНИЕ: превью советное — JS RegExp принимает lookahead, который Go RE2
 * отвергнет; авторитетная проверка и разрешение состава — на сервере.
 */
export interface GroupPreviewResult {
	/** Сколько серверов попадёт в группу (по клиентской оценке). */
	count: number;
	/** Имена попавших серверов (для отображения списком). */
	labels: string[];
	/** filterInclude не скомпилировался в JS (сервер отклонит с точной ошибкой). */
	invalidInclude: boolean;
	/** filterExclude не скомпилировался в JS. */
	invalidExclude: boolean;
}

function compileSoft(pattern: string): { re: RegExp | null; invalid: boolean } {
	if (!pattern) return { re: null, invalid: false };
	try {
		return { re: new RegExp(pattern), invalid: false };
	} catch {
		return { re: null, invalid: true };
	}
}

/**
 * Клиентское зеркало серверного разрешения состава группы: подписки в
 * порядке useSubscriptionIds (дубликаты схлопываются), только существующие
 * и включённые, члены в сохранённом порядке, фильтр по имени сервера.
 * Некомпилируемый шаблон трактуется как «без ограничения» + флаг invalid.
 */
export function resolveGroupPreview(
	subs: Subscription[],
	useSubscriptionIds: string[],
	filterInclude: string,
	filterExclude: string,
): GroupPreviewResult {
	const include = compileSoft(filterInclude);
	const exclude = compileSoft(filterExclude);
	const byId = new Map(subs.map((s) => [s.id, s]));
	const seen = new Set<string>();
	const labels: string[] = [];
	for (const id of useSubscriptionIds) {
		if (seen.has(id)) continue;
		seen.add(id);
		const sub = byId.get(id);
		if (!sub || !sub.enabled) continue;
		for (const m of sub.members ?? []) {
			const label = m.label ?? '';
			if (include.re && !include.re.test(label)) continue;
			if (exclude.re && exclude.re.test(label)) continue;
			labels.push(label || `${m.server}:${m.port}`);
		}
	}
	return {
		count: labels.length,
		labels,
		invalidInclude: include.invalid,
		invalidExclude: exclude.invalid,
	};
}
