<script lang="ts">
	import type { Subscription } from '$lib/types';
	import { api } from '$lib/api/client';

	interface Props {
		subscription: Subscription;
		onUpdated: () => void;
	}
	let { subscription, onUpdated }: Props = $props();

	let updating = $state(false);

	async function toggleDefaultRoute(): Promise<void> {
		updating = true;
		try {
			await api.setSubscriptionDefaultRoute(subscription.id, !subscription.isDefaultRoute);
			onUpdated();
		} finally {
			updating = false;
		}
	}
</script>

<div class="grid">
	<div class="row">
		<div class="lbl">Selector tag</div>
		<div class="val mono">{subscription.selectorTag}</div>
	</div>
	<div class="row">
		<div class="lbl">Inbound tag</div>
		<div class="val mono">{subscription.inboundTag}</div>
	</div>
	<div class="row">
		<div class="lbl">Listen port</div>
		<div class="val mono">127.0.0.1:{subscription.listenPort}</div>
	</div>
	<div class="row">
		<div class="lbl">Серверов</div>
		<div class="val">{subscription.memberTags.length}</div>
	</div>
	<div class="row">
		<div class="lbl">Активный сервер</div>
		<div class="val mono">{subscription.activeMember || '—'}</div>
	</div>
	{#if subscription.orphanTags.length > 0}
		<div class="row warn">
			<div class="lbl">Orphan</div>
			<div class="val">{subscription.orphanTags.length} (не пришли в последнем refresh)</div>
		</div>
	{/if}
	<div class="row toggle">
		<div class="lbl">Маршрут по умолчанию</div>
		<button
			class="btn {subscription.isDefaultRoute ? 'on' : ''}"
			disabled={updating}
			onclick={toggleDefaultRoute}
		>
			{subscription.isDefaultRoute ? 'ON' : 'OFF'}
		</button>
	</div>
</div>

<style>
	.grid { display: flex; flex-direction: column; gap: 0.5rem; }
	.row { display: flex; padding: 0.5rem 0; border-bottom: 1px solid var(--color-border); }
	.row.warn { color: #d29922; }
	.lbl { width: 180px; color: var(--color-text-muted); font-size: 0.85rem; }
	.val { color: var(--color-text-primary); font-size: 0.85rem; flex: 1; }
	.mono { font-family: var(--font-mono, ui-monospace, monospace); }
	.btn {
		padding: 0.3rem 1rem;
		border-radius: 4px;
		background: var(--color-bg-tertiary);
		color: var(--color-text-muted);
		border: 1px solid var(--color-border);
		cursor: pointer;
	}
	.btn.on { background: #238636; color: white; border-color: #2ea043; }
</style>
