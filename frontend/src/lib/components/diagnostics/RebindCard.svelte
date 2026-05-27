<script lang="ts">
	import type { DnsRebind } from '$lib/types';
	interface Props { rebind: DnsRebind; }
	let { rebind }: Props = $props();
</script>

<div class="rebind">
	<div class="head">
		<span class="label">Rebind-защита</span>
		<span class="status" class:on={rebind.enabled}>{rebind.enabled ? 'включена' : 'выключена'}</span>
	</div>
	<div class="kv">
		<div>
			<div class="k">Защищённые сети</div>
			<div class="tags">
				{#each rebind.nets as n}<span class="tag">{n}</span>{:else}<span class="muted">—</span>{/each}
			</div>
		</div>
		<div>
			<div class="k">Исключения</div>
			<div class="tags">
				{#each rebind.excludes as e}<span class="tag">{e}</span>{:else}<span class="muted">—</span>{/each}
			</div>
		</div>
	</div>
</div>

<style>
	.head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
	.label { font-size: 11px; font-weight: 700; letter-spacing: .06em; text-transform: uppercase; color: var(--text-muted); }
	.status { display: inline-flex; align-items: center; gap: 6px; font-weight: 600; font-size: 13px; color: var(--text-muted); }
	.status.on { color: var(--success); }
	.status.on::before { content: ''; width: 7px; height: 7px; border-radius: 50%; background: var(--success); }
	.kv { display: flex; gap: 28px; flex-wrap: wrap; }
	.kv .k { font-size: 10px; font-weight: 600; text-transform: uppercase; letter-spacing: .04em; color: var(--text-muted); margin-bottom: 6px; }
	.tags { display: flex; flex-wrap: wrap; gap: 6px; }
	.tag { font-family: ui-monospace, monospace; font-size: 12px; background: color-mix(in srgb, var(--text-muted) 12%, transparent); border-radius: 6px; padding: 3px 9px; }
	.muted { color: var(--text-muted); }
</style>
