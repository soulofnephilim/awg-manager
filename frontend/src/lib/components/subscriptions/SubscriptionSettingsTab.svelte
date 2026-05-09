<script lang="ts">
	import {
		DEFAULT_SUBSCRIPTION_URLTEST,
		type Subscription,
		type SubscriptionMode,
	} from '$lib/types';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import HeadersTextarea from './HeadersTextarea.svelte';
	import { parseHeadersText, serializeHeaders } from './headersParser';
	import { Button, Dropdown } from '$lib/components/ui';
	import { untrack } from 'svelte';

	interface Props {
		subscription: Subscription;
		onUpdated: () => void;
	}
	let { subscription, onUpdated }: Props = $props();

	let label = $state(untrack(() => subscription.label));
	let url = $state(untrack(() => subscription.url));
	let headersText = $state(untrack(() => serializeHeaders(subscription.headers)));
	let refreshHoursStr = $state(untrack(() => String(subscription.refreshHours)));
	let refreshHours = $state(untrack(() => subscription.refreshHours));
	let enabled = $state(untrack(() => subscription.enabled));
	let mode = $state<SubscriptionMode>(untrack(() => subscription.mode ?? 'selector'));
	let utUrl = $state(
		untrack(() => subscription.urlTest?.url ?? DEFAULT_SUBSCRIPTION_URLTEST.url),
	);
	let utIntervalSec = $state(
		untrack(() => subscription.urlTest?.intervalSec ?? DEFAULT_SUBSCRIPTION_URLTEST.intervalSec),
	);
	let utToleranceMs = $state(
		untrack(() => subscription.urlTest?.toleranceMs ?? DEFAULT_SUBSCRIPTION_URLTEST.toleranceMs),
	);
	let saving = $state(false);
	let confirmDelete = $state(false);
	let deleting = $state(false);

	// Re-sync form state when subscription prop changes after parent reload.
	$effect(() => {
		label = subscription.label;
		url = subscription.url;
		headersText = serializeHeaders(subscription.headers);
		refreshHoursStr = String(subscription.refreshHours);
		refreshHours = subscription.refreshHours;
		enabled = subscription.enabled;
		mode = subscription.mode ?? 'selector';
		utUrl = subscription.urlTest?.url ?? DEFAULT_SUBSCRIPTION_URLTEST.url;
		utIntervalSec =
			subscription.urlTest?.intervalSec ?? DEFAULT_SUBSCRIPTION_URLTEST.intervalSec;
		utToleranceMs =
			subscription.urlTest?.toleranceMs ?? DEFAULT_SUBSCRIPTION_URLTEST.toleranceMs;
	});

	$effect(() => {
		refreshHours = parseInt(refreshHoursStr, 10) || 0;
	});

	const refreshOptions = [
		{ value: '0', label: 'Только вручную' },
		{ value: '1', label: 'Каждый час' },
		{ value: '6', label: 'Каждые 6 часов' },
		{ value: '12', label: 'Каждые 12 часов' },
		{ value: '24', label: 'Раз в сутки' },
		{ value: '168', label: 'Раз в неделю' },
	];

	async function save(): Promise<void> {
		saving = true;
		try {
			const patch: Parameters<typeof api.updateSubscription>[1] = {
				label,
				enabled,
				mode,
				urlTest:
					mode === 'urltest'
						? { url: utUrl, intervalSec: utIntervalSec, toleranceMs: utToleranceMs }
						: undefined,
			};
			if (!subscription.isInline) {
				patch.url = url;
				patch.headers = parseHeadersText(headersText);
				patch.refreshHours = refreshHours;
			}
			await api.updateSubscription(subscription.id, patch);
			onUpdated();
		} finally {
			saving = false;
		}
	}

	async function doDelete(): Promise<void> {
		deleting = true;
		try {
			await api.deleteSubscription(subscription.id);
			goto('/');
		} finally {
			deleting = false;
		}
	}
</script>

<form
	class="form"
	onsubmit={(e) => {
		e.preventDefault();
		save();
	}}
>
	<label><span>Название</span><input bind:value={label} /></label>
	{#if subscription.isInline}
		<div class="inline-info">
			<div class="inline-badge">Список вручную</div>
			<div class="inline-summary">
				{subscription.members.length === 1
					? '1 сервер'
					: subscription.members.length < 5
						? `${subscription.members.length} сервера`
						: `${subscription.members.length} серверов`}
				· редактирование во вкладке «Серверы»
			</div>
		</div>
	{:else}
		<label><span>URL</span><input bind:value={url} /></label>
		<HeadersTextarea bind:value={headersText} />
		<Dropdown
			label="Авто-обновление"
			bind:value={refreshHoursStr}
			options={refreshOptions}
		/>
	{/if}

	<div class="mode-section">
		<span class="mode-label">Режим выбора сервера</span>
		<div class="mode-grid" role="radiogroup" aria-label="Режим выбора сервера">
			<button
				type="button"
				role="radio"
				aria-checked={mode === 'selector'}
				class="mode-card"
				class:selected={mode === 'selector'}
				onclick={() => (mode = 'selector')}
			>
				<div class="mode-title">Ручной выбор</div>
				<div class="mode-desc">
					Сервер переключается вручную из списка.
				</div>
				{#if mode === 'selector'}
					<span class="mode-check" aria-hidden="true">
						<svg viewBox="0 0 24 24"><polyline points="20 6 9 17 4 12" /></svg>
					</span>
				{/if}
			</button>
			<button
				type="button"
				role="radio"
				aria-checked={mode === 'urltest'}
				class="mode-card"
				class:selected={mode === 'urltest'}
				onclick={() => (mode = 'urltest')}
			>
				<div class="mode-title">Автовыбор по скорости</div>
				<div class="mode-desc">
					Sing-box сам пингует серверы и держит самый быстрый.
				</div>
				{#if mode === 'urltest'}
					<span class="mode-check" aria-hidden="true">
						<svg viewBox="0 0 24 24"><polyline points="20 6 9 17 4 12" /></svg>
					</span>
				{/if}
			</button>
		</div>
		{#if mode === 'urltest'}
			<div class="urltest-block">
				<label>
					<span>URL для проверки</span>
					<input type="url" bind:value={utUrl} placeholder={DEFAULT_SUBSCRIPTION_URLTEST.url} />
				</label>
				<div class="ut-row">
					<label class="ut-col">
						<span>Интервал, сек</span>
						<input type="number" min="10" max="3600" bind:value={utIntervalSec} />
					</label>
					<label class="ut-col">
						<span>Допуск, мс</span>
						<input type="number" min="0" max="2000" bind:value={utToleranceMs} />
					</label>
				</div>
			</div>
		{/if}
	</div>

	<label class="chk"><input type="checkbox" bind:checked={enabled} /> Включена</label>
	<div class="actions">
		<Button type="submit" variant="primary" disabled={saving} loading={saving}>
			{saving ? 'Сохраняем...' : 'Сохранить'}
		</Button>
	</div>
</form>

<div class="danger-zone">
	{#if !confirmDelete}
		<Button variant="danger" onclick={() => (confirmDelete = true)}>Удалить подписку</Button>
	{:else}
		<div>Удалить подписку и все её ресурсы (sing-box outbound'ы, NDMS Proxy)?</div>
		<div class="confirm-actions">
			<Button variant="danger" disabled={deleting} loading={deleting} onclick={doDelete}>
				Удалить
			</Button>
			<Button variant="ghost" onclick={() => (confirmDelete = false)}>Отмена</Button>
		</div>
	{/if}
</div>

<style>
	.form { display: flex; flex-direction: column; gap: 0.7rem; max-width: 640px; }
	.form label { display: flex; flex-direction: column; gap: 0.3rem; }
	.form label.chk { flex-direction: row; align-items: center; gap: 0.5rem; }
	input {
		padding: 0.45rem 0.6rem;
		border: 1px solid var(--color-border);
		border-radius: 4px;
		background: var(--color-bg-primary);
		color: var(--color-text-primary);
	}
	.actions { display: flex; justify-content: flex-end; }
	.danger-zone {
		margin-top: 1.5rem;
		padding-top: 1rem;
		border-top: 1px solid var(--color-border);
	}
	.confirm-actions { display: flex; gap: 0.5rem; flex-wrap: wrap; margin-top: 0.5rem; }

	.mode-section { display: flex; flex-direction: column; gap: 0.5rem; }
	.mode-label { font-size: 0.85rem; color: var(--color-text-muted); }
	.mode-grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.5rem;
	}
	.mode-card {
		position: relative;
		text-align: left;
		padding: 0.6rem 0.75rem;
		background: var(--color-bg-primary);
		border: 1px solid var(--color-border);
		border-radius: 6px;
		color: var(--color-text-primary);
		cursor: pointer;
		font: inherit;
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		transition: border-color 120ms, background 120ms;
	}
	.mode-card:hover { border-color: var(--color-text-muted); }
	.mode-card.selected {
		border-color: var(--color-primary, #3b82f6);
		background: rgba(59, 130, 246, 0.06);
	}
	.mode-card:focus-visible {
		outline: 2px solid var(--color-primary, #3b82f6);
		outline-offset: 2px;
	}
	.mode-title { font-weight: 500; font-size: 0.85rem; }
	.mode-desc { font-size: 0.72rem; color: var(--color-text-muted); line-height: 1.35; }
	.mode-check {
		position: absolute;
		top: 0.5rem;
		right: 0.5rem;
		width: 14px;
		height: 14px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		color: var(--color-primary, #3b82f6);
	}
	.mode-check svg { width: 12px; height: 12px; fill: none; stroke: currentColor; stroke-width: 3; }
	.urltest-block {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 0.6rem 0.8rem;
		background: var(--color-bg-secondary, var(--color-bg-primary));
		border: 1px dashed var(--color-border);
		border-radius: 4px;
	}
	.ut-row { display: flex; gap: 0.6rem; }
	.ut-col { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 0.3rem; }
	@media (max-width: 480px) {
		.mode-grid { grid-template-columns: 1fr; }
		.ut-row { flex-direction: column; }
	}
	.inline-info {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		padding: 0.6rem 0.8rem;
		background: var(--color-bg-secondary, var(--color-bg-primary));
		border: 1px dashed var(--color-border);
		border-radius: 4px;
	}
	.inline-badge {
		display: inline-flex;
		align-self: flex-start;
		padding: 0.15rem 0.5rem;
		background: var(--color-primary, #3b82f6);
		color: white;
		border-radius: 3px;
		font-size: 0.72rem;
		font-weight: 500;
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}
	.inline-summary {
		font-size: 0.78rem;
		color: var(--color-text-muted);
	}
</style>
