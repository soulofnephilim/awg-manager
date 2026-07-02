<script lang="ts">
	import type { Snippet } from 'svelte';
	import { onDestroy, onMount } from 'svelte';
	import { Button } from '$lib/components/ui';

	interface Props {
		onAwg: () => void;
		onSingboxSingle?: () => void;
		onSingboxGroup?: () => void;
		onSingboxSubscription?: () => void;
		showSingbox?: boolean;
		triggerIcon: Snippet;
		triggerLabel?: string;
	}

	let {
		onAwg,
		onSingboxSingle,
		onSingboxGroup,
		onSingboxSubscription,
		showSingbox = true,
		triggerIcon,
		triggerLabel = 'Создать',
	}: Props = $props();

	let open = $state(false);
	let rootEl = $state<HTMLDivElement | null>(null);

	function close(): void {
		open = false;
	}

	function toggle(event: MouseEvent): void {
		event.stopPropagation();
		open = !open;
	}

	function pick(action: () => void): void {
		close();
		action();
	}

	function handleDocumentClick(event: MouseEvent): void {
		if (!open || !rootEl) return;
		if (!rootEl.contains(event.target as Node)) close();
	}

	function handleDocumentKeydown(event: KeyboardEvent): void {
		if (open && event.key === 'Escape') close();
	}

	onMount(() => {
		document.addEventListener('click', handleDocumentClick);
		document.addEventListener('keydown', handleDocumentKeydown);
	});

	onDestroy(() => {
		document.removeEventListener('click', handleDocumentClick);
		document.removeEventListener('keydown', handleDocumentKeydown);
	});
</script>

<div class="tunnel-create-menu" bind:this={rootEl} aria-haspopup="menu" aria-expanded={open}>
	<Button variant="primary" size="md" onclick={toggle} iconBefore={triggerIcon}>
		{triggerLabel}
		{#snippet iconAfter()}
			<svg width="10" height="10" viewBox="0 0 10 10" fill="currentColor" aria-hidden="true">
				<path d="M2 4l3 3 3-3" />
			</svg>
		{/snippet}
	</Button>

	{#if open}
		<div class="tunnel-create-menu-panel" role="menu">
			<button type="button" class="tunnel-create-menu-item" role="menuitem" onclick={() => pick(onAwg)}>
				<span class="tunnel-create-menu-item-title">AmneziaWG туннель</span>
				<span class="tunnel-create-menu-item-desc">NativeWG или Kernel</span>
			</button>

			{#if showSingbox}
				<div class="tunnel-create-menu-sep" role="separator"></div>
				{#if onSingboxSingle}
					<button
						type="button"
						class="tunnel-create-menu-item"
						role="menuitem"
						onclick={() => pick(onSingboxSingle)}
					>
						<span class="tunnel-create-menu-item-title">Один сервер</span>
						<span class="tunnel-create-menu-item-desc">Share-link → sing-box туннель</span>
					</button>
				{/if}
				{#if onSingboxGroup}
					<button
						type="button"
						class="tunnel-create-menu-item"
						role="menuitem"
						onclick={() => pick(onSingboxGroup)}
					>
						<span class="tunnel-create-menu-item-title">Группа серверов</span>
						<span class="tunnel-create-menu-item-desc">Несколько ссылок в одной группе</span>
					</button>
				{/if}
				{#if onSingboxSubscription}
					<button
						type="button"
						class="tunnel-create-menu-item"
						role="menuitem"
						onclick={() => pick(onSingboxSubscription)}
					>
						<span class="tunnel-create-menu-item-title">Подписка по URL</span>
						<span class="tunnel-create-menu-item-desc">Автообновляемый список серверов</span>
					</button>
				{/if}
			{/if}
		</div>
	{/if}
</div>

<style>
	.tunnel-create-menu {
		position: relative;
		display: inline-block;
	}

	.tunnel-create-menu-panel {
		position: absolute;
		top: calc(100% + 4px);
		right: 0;
		z-index: 30;
		min-width: 15rem;
		padding: 0.25rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-sm);
		background: var(--color-bg-secondary);
		box-shadow: var(--shadow-lg, 0 8px 24px rgba(0, 0, 0, 0.28));
	}

	.tunnel-create-menu-item {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 0.1rem;
		width: 100%;
		padding: 0.5rem 0.625rem;
		border: none;
		border-radius: calc(var(--radius-sm) - 2px);
		background: transparent;
		color: inherit;
		font: inherit;
		text-align: left;
		cursor: pointer;
	}

	.tunnel-create-menu-item:hover {
		background: var(--color-bg-hover);
	}

	.tunnel-create-menu-item-title {
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--color-text-primary);
	}

	.tunnel-create-menu-item-desc {
		font-size: 0.6875rem;
		color: var(--color-text-muted);
		line-height: 1.35;
	}

	.tunnel-create-menu-sep {
		height: 1px;
		margin: 0.2rem 0.35rem;
		background: var(--color-border);
	}

	@media (max-width: 640px) {
		.tunnel-create-menu {
			display: block;
			width: 100%;
		}

		.tunnel-create-menu :global(.btn) {
			width: 100%;
			justify-content: center;
		}
	}
</style>
