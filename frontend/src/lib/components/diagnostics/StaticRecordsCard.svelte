<script lang="ts">
	import type { DnsStaticRecord } from '$lib/types';
	interface Props { records: DnsStaticRecord[]; }
	let { records }: Props = $props();
	let open = $state(false);
</script>

<div class="static" class:open>
	<button type="button" class="head" onclick={() => (open = !open)}>
		<span class="chev">›</span>
		<span class="title">Статические записи</span>
		<span class="count">{records.length}</span>
	</button>
	{#if open}
		<table>
			<thead><tr><th>Хост</th><th>Тип</th><th>Значение</th><th class="num">Flag</th></tr></thead>
			<tbody>
				{#each records as r}
					<tr>
						<td class="mono">{r.host}</td>
						<td><span class="type" class:aaaa={r.type === 'AAAA'}>{r.type}</span></td>
						<td class="mono muted">{r.value}</td>
						<td class="num">{r.flag}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
</div>

<style>
	.head { display: flex; align-items: center; gap: 8px; width: 100%; background: none; border: none; cursor: pointer; color: inherit; font: inherit; padding: 0; }
	.chev { color: var(--text-muted); transition: transform .15s; }
	.static.open .chev { transform: rotate(90deg); }
	.title { font-weight: 600; }
	.count { font-family: ui-monospace, monospace; font-size: 11px; color: var(--text-muted); background: color-mix(in srgb, var(--text-muted) 14%, transparent); border-radius: 5px; padding: 1px 7px; }
	table { width: 100%; border-collapse: collapse; margin-top: 12px; }
	th { font-size: 11px; font-weight: 600; color: var(--text-muted); text-transform: uppercase; letter-spacing: .04em; padding: 0 10px 8px 0; text-align: left; }
	td { padding: 8px 10px 8px 0; border-top: 1px solid var(--border-soft, var(--border)); font-size: 13px; }
	.mono { font-family: ui-monospace, monospace; }
	.muted { color: var(--text-muted); }
	.num { text-align: right; font-family: ui-monospace, monospace; }
	th.num { text-align: right; }
	.type { font-family: ui-monospace, monospace; font-size: 11px; font-weight: 650; color: var(--success); }
	.type.aaaa { color: var(--primary); }
</style>
