<script lang="ts">
	import type { DnsProxy } from '$lib/types';
	interface Props { proxy: DnsProxy; open?: boolean; }
	let { proxy, open = false }: Props = $props();
	let expanded = $state(open);

	const cachePct = $derived(Math.round((proxy.stat.cacheHitRatio || 0) * 100));
	const rawChip = $derived(
		proxy.name !== 'System' && proxy.displayName !== proxy.name ? proxy.name : ''
	);
</script>

<div class="pol" class:open={expanded}>
	<button type="button" class="pol-head" onclick={() => (expanded = !expanded)}>
		<span class="chev">›</span>
		<span class="pol-name">
			{proxy.displayName}
			{#if rawChip}<span class="chip">{rawChip}</span>{/if}
		</span>
		<span class="pol-port">:{proxy.tcpPort}</span>
		<span class="pol-metrics">
			<span class="metric"><span class="v">{proxy.stat.totalRequests}</span><span class="k">запросов</span></span>
			<span class="metric"><span class="v faint">{proxy.stat.proxyRequestsSent}</span><span class="k">proxy</span></span>
			<span class="cache">
				<span class="bar"><span style="width:{cachePct}%"></span></span>
				<span class="metric"><span class="v">{cachePct}%</span><span class="k">cache</span></span>
			</span>
		</span>
	</button>
	{#if expanded}
		<div class="pol-body">
			<table>
				<thead>
					<tr><th>Сервер</th><th class="num">Отпр</th><th class="num">Получ</th><th class="num">NX</th><th class="num">Медиана</th><th class="num">Среднее</th><th class="num">Rank</th></tr>
				</thead>
				<tbody>
					{#each proxy.upstreams as u}
						<tr>
							<td class="mono">{u.address}</td>
							<td class="num">{u.rSent}</td>
							<td class="num">{u.aRcvd}</td>
							<td class="num">{u.nxRcvd}</td>
							<td class="num">{u.medResp || '—'}</td>
							<td class="num">{u.avgResp || '—'}</td>
							<td class="num"><span class="rank">{u.rank}</span></td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<style>
	.pol { border: 1px solid var(--border-soft, var(--border)); border-radius: 10px; margin-bottom: 8px; overflow: hidden; }
	.pol-head { display: flex; align-items: center; gap: 14px; width: 100%; padding: 11px 14px; background: none; border: none; cursor: pointer; text-align: left; color: inherit; font: inherit; }
	.pol-head:hover { background: var(--surface-hover, rgba(127,127,127,.06)); }
	.chev { color: var(--text-muted); transition: transform .15s; }
	.pol.open .chev { transform: rotate(90deg); }
	.pol-name { font-weight: 600; display: flex; align-items: center; gap: 8px; min-width: 150px; }
	.chip { font-family: ui-monospace, monospace; font-size: 10px; font-weight: 600; color: var(--text-muted); background: color-mix(in srgb, var(--text-muted) 14%, transparent); border-radius: 5px; padding: 1px 6px; }
	.pol-port { font-family: ui-monospace, monospace; font-size: 12px; color: var(--text-muted); min-width: 60px; }
	.pol-metrics { margin-left: auto; display: flex; align-items: center; gap: 18px; }
	.metric { display: flex; flex-direction: column; align-items: flex-end; }
	.metric .v { font-family: ui-monospace, monospace; font-weight: 600; font-size: 14px; }
	.metric .k { font-size: 10px; color: var(--text-muted); text-transform: uppercase; letter-spacing: .04em; }
	.faint { opacity: .65; }
	.cache { display: flex; align-items: center; gap: 8px; }
	.bar { width: 54px; height: 6px; border-radius: 999px; background: color-mix(in srgb, var(--text-muted) 18%, transparent); overflow: hidden; }
	.bar > span { display: block; height: 100%; background: var(--primary); }
	.pol-body { padding: 4px 14px 12px; border-top: 1px solid var(--border-soft, var(--border)); }
	.pol-body table { width: 100%; border-collapse: collapse; margin-top: 6px; }
	.pol-body th { font-size: 11px; font-weight: 600; color: var(--text-muted); text-transform: uppercase; letter-spacing: .04em; padding: 4px 10px 6px 0; text-align: left; }
	.pol-body td { padding: 6px 10px 6px 0; border-top: 1px solid var(--border-soft, var(--border)); }
	.num { text-align: right; font-family: ui-monospace, monospace; font-variant-numeric: tabular-nums; }
	th.num { text-align: right; }
	.mono { font-family: ui-monospace, monospace; font-size: 13px; }
	.rank { font-family: ui-monospace, monospace; font-size: 11px; font-weight: 650; color: var(--primary); background: color-mix(in srgb, var(--primary) 14%, transparent); border-radius: 5px; padding: 1px 7px; }
</style>
