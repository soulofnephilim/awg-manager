<script lang="ts">
	import type { DnsStaticRecord } from '$lib/types';
	import { Badge } from '$lib/components/ui';
	interface Props { records: DnsStaticRecord[]; }
	let { records }: Props = $props();
	let open = $state(false);
</script>

<div class="static" class:open>
	<button type="button" class="head" onclick={() => (open = !open)}>
		<span class="chev">›</span>
		<span class="title">Статические записи</span>
		<Badge variant="muted" size="sm" mono>{records.length}</Badge>
	</button>
	{#if open}
		<table>
			<thead><tr><th>Хост</th><th>Тип</th><th>Значение</th><th class="num">Flag</th></tr></thead>
			<tbody>
				{#each records as r}
					<tr>
						<td class="mono">{r.host}</td>
						<td><Badge variant={r.type === 'AAAA' ? 'info' : 'success'} size="sm" mono>{r.type}</Badge></td>
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
	table { width: 100%; border-collapse: collapse; margin-top: 12px; }
	th { font-size: 11px; font-weight: 600; color: var(--text-muted); text-transform: uppercase; letter-spacing: .04em; padding: 0 10px 8px 0; text-align: left; }
	td { padding: 8px 10px 8px 0; border-top: 1px solid var(--border-soft, var(--border)); font-size: 13px; }
	.mono { font-family: ui-monospace, monospace; }
	.muted { color: var(--text-muted); }
	.num { text-align: right; font-family: ui-monospace, monospace; }
	th.num { text-align: right; }
</style>
