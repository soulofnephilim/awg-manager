<script lang="ts">
	import type { DnsUpstream } from '$lib/types';
	import { Badge } from '$lib/components/ui';
	import type { BadgeVariant } from '$lib/components/ui';
	interface Props { upstreams: DnsUpstream[]; }
	let { upstreams }: Props = $props();

	function encVariant(e: string): BadgeVariant {
		if (e === 'DoT') return 'success';
		if (e === 'DoH') return 'accent';
		return 'muted';
	}
	function scopeLabel(s: string): string {
		return s === 'all' ? 'все' : `.${s}`;
	}
</script>

<table class="up-table">
	<thead>
		<tr><th>Сервер</th><th>Шифрование</th><th>Хост / SNI</th><th>Домены</th></tr>
	</thead>
	<tbody>
		{#each upstreams as u}
			<tr>
				<td class="mono">{u.address}{#if u.port}<span class="faint">:{u.port}</span>{/if}</td>
				<td><Badge variant={encVariant(u.encryption)} size="sm">{u.encryption}</Badge></td>
				<td class="muted">{u.sni || '—'}</td>
				<td><span class="scope" class:scope-ru={u.scope !== 'all'}>{scopeLabel(u.scope)}</span></td>
			</tr>
		{/each}
	</tbody>
</table>

<style>
	.up-table { width: 100%; border-collapse: collapse; }
	.up-table th { font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: .04em; color: var(--text-muted); text-align: left; padding: 0 10px 8px 0; }
	.up-table td { padding: 9px 10px 9px 0; border-top: 1px solid var(--border-soft, var(--border)); }
	.up-table tr:first-child td { border-top: none; }
	.mono { font-family: ui-monospace, monospace; font-size: 13px; }
	.faint { color: var(--text-muted); opacity: .7; }
	.muted { color: var(--text-muted); }
	.scope { font-size: 12px; color: var(--text-muted); }
	.scope-ru { color: var(--accent); font-weight: 600; }
</style>
