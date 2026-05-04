<script lang="ts">
	import type { Subscription } from '$lib/types';
	import { api } from '$lib/api/client';
	import { Button } from '$lib/components/ui';

	interface Props {
		subscription: Subscription;
		onUpdated: () => void;
	}
	let { subscription, onUpdated }: Props = $props();

	let refreshing = $state(false);
	let switching = $state<string | null>(null);
	let lastError = $state('');

	async function refresh(): Promise<void> {
		refreshing = true;
		lastError = '';
		try {
			await api.refreshSubscription(subscription.id);
			onUpdated();
		} catch (e) {
			lastError = e instanceof Error ? e.message : 'Не удалось обновить';
		} finally {
			refreshing = false;
		}
	}

	async function pickActive(memberTag: string): Promise<void> {
		if (memberTag === subscription.activeMember) return;
		switching = memberTag;
		lastError = '';
		try {
			await api.setSubscriptionActiveMember(subscription.id, memberTag);
			onUpdated();
		} catch (e) {
			lastError = e instanceof Error ? e.message : 'Не удалось переключить';
		} finally {
			switching = null;
		}
	}
</script>

<div class="head">
	<div class="info">
		<div class="lbl">Selector</div>
		<div class="val mono">{subscription.selectorTag}</div>
	</div>
	<Button variant="primary" size="sm" disabled={refreshing} loading={refreshing} onclick={refresh}>
		{refreshing ? 'Обновляем...' : 'Обновить сейчас'}
	</Button>
</div>

{#if lastError}
	<div class="err">{lastError}</div>
{/if}

{#if subscription.memberTags.length === 0}
	<div class="empty">Подписка ещё не загружена. Нажмите «Обновить сейчас».</div>
{:else}
	<div class="hint">Выберите активный сервер. Selector направит трафик в выбранный outbound.</div>
	<div class="list">
		{#each subscription.memberTags as tag (tag)}
			{@const isActive = tag === subscription.activeMember}
			{@const isSwitching = switching === tag}
			<button
				type="button"
				class="row"
				class:active={isActive}
				class:switching={isSwitching}
				disabled={switching !== null}
				onclick={() => pickActive(tag)}
			>
				<span class="radio" class:on={isActive}></span>
				<span class="tag mono">{tag}</span>
				{#if isActive}<span class="badge">активен</span>{/if}
				{#if isSwitching}<span class="badge spin">переключаем...</span>{/if}
			</button>
		{/each}
	</div>
{/if}

{#if subscription.orphanTags.length > 0}
	<div class="orphans">
		<div class="lbl warn">Orphan ({subscription.orphanTags.length})</div>
		<div class="hint">Серверы из прошлой версии подписки, не вернувшиеся при последнем refresh.</div>
		<div class="list">
			{#each subscription.orphanTags as tag (tag)}
				<div class="row orphan">
					<span class="tag mono">{tag}</span>
				</div>
			{/each}
		</div>
	</div>
{/if}

<style>
	.head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 1rem;
		margin-bottom: 1rem;
	}
	.info { display: flex; flex-direction: column; gap: 0.2rem; }
	.lbl { font-size: 0.7rem; color: var(--color-text-muted); text-transform: uppercase; letter-spacing: 0.5px; }
	.lbl.warn { color: #d29922; }
	.val { color: var(--color-text-primary); font-size: 0.85rem; }
	.err { color: #f85149; font-size: 0.85rem; margin-bottom: 0.6rem; }
	.hint { color: var(--color-text-muted); font-size: 0.82rem; margin-bottom: 0.6rem; }
	.empty {
		padding: 2rem;
		text-align: center;
		color: var(--color-text-muted);
		border: 1px dashed var(--color-border);
		border-radius: 6px;
	}
	.list {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	.row {
		display: flex;
		align-items: center;
		gap: 0.7rem;
		padding: 0.6rem 0.8rem;
		background: var(--color-bg-secondary);
		border: 1px solid var(--color-border);
		border-radius: 6px;
		font: inherit;
		text-align: left;
		cursor: pointer;
		color: var(--color-text-primary);
	}
	.row:hover:not(:disabled):not(.active) { border-color: var(--color-accent); }
	.row.active {
		border-color: #3fb950;
		background: rgba(63, 185, 80, 0.08);
	}
	.row.switching { opacity: 0.7; cursor: wait; }
	.row:disabled { cursor: wait; }
	.row.orphan { color: var(--color-text-muted); cursor: default; opacity: 0.7; }
	.radio {
		width: 14px;
		height: 14px;
		border-radius: 50%;
		border: 2px solid var(--color-border);
		flex-shrink: 0;
	}
	.radio.on {
		border-color: #3fb950;
		background: #3fb950;
		box-shadow: inset 0 0 0 3px var(--color-bg-secondary);
	}
	.tag { flex: 1; font-size: 0.82rem; }
	.mono { font-family: var(--font-mono, ui-monospace, monospace); }
	.badge {
		font-size: 0.72rem;
		padding: 0.15rem 0.5rem;
		border-radius: 999px;
		background: rgba(63, 185, 80, 0.15);
		color: #3fb950;
	}
	.badge.spin { background: rgba(88, 166, 255, 0.15); color: var(--color-accent); }
	.orphans { margin-top: 1.5rem; padding-top: 1rem; border-top: 1px solid var(--color-border); }
</style>
