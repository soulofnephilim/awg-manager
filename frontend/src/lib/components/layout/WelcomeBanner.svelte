<script lang="ts">
	import { onMount } from 'svelte';
	import { usageLevel } from '$lib/stores/settings';

	const STORAGE_KEY = 'awgm.welcomeBannerDismissed';

	let dismissed = $state(true);

	onMount(() => {
		dismissed = localStorage.getItem(STORAGE_KEY) === '1';
	});

	const visible = $derived($usageLevel === 'basic' && !dismissed);

	function dismiss() {
		localStorage.setItem(STORAGE_KEY, '1');
		dismissed = true;
	}
</script>

{#if visible}
	<div class="welcome-banner" role="status">
		<div class="banner-icon" aria-hidden="true">
			<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
				<circle cx="12" cy="12" r="10" />
				<line x1="12" y1="8" x2="12" y2="12" />
				<circle cx="12" cy="16" r="0.8" fill="currentColor" />
			</svg>
		</div>
		<div class="banner-body">
			<strong>Вы в Базовом режиме</strong>
			<p>
				Доступны только туннели и диагностика. Чтобы открыть серверы, маршрутизацию,
				мониторинг и другие возможности — выберите более высокий уровень в
				<a href="/settings">Настройках</a>.
			</p>
		</div>
		<button
			type="button"
			class="banner-close"
			aria-label="Скрыть подсказку"
			onclick={dismiss}
		>
			<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
				<line x1="18" y1="6" x2="6" y2="18" />
				<line x1="6" y1="6" x2="18" y2="18" />
			</svg>
		</button>
	</div>
{/if}

<style>
	.welcome-banner {
		display: flex;
		align-items: flex-start;
		gap: 0.75rem;
		padding: 0.875rem 1rem;
		margin-bottom: 1rem;
		background: var(--color-info-tint, var(--color-bg-tertiary));
		border: 1px solid var(--color-info, var(--color-border-strong));
		border-radius: var(--radius-md);
		color: var(--color-text-primary);
	}
	.banner-icon {
		flex-shrink: 0;
		color: var(--color-info, var(--color-accent));
	}
	.banner-icon svg {
		width: 20px;
		height: 20px;
	}
	.banner-body {
		flex: 1;
	}
	.banner-body strong {
		display: block;
		margin-bottom: 0.125rem;
	}
	.banner-body p {
		margin: 0;
		font-size: 0.875rem;
		color: var(--color-text-secondary);
	}
	.banner-body a {
		color: var(--color-accent);
		text-decoration: underline;
	}
	.banner-close {
		flex-shrink: 0;
		background: transparent;
		border: 0;
		color: var(--color-text-muted);
		cursor: pointer;
		padding: 0.25rem;
		border-radius: var(--radius-sm);
	}
	.banner-close:hover {
		color: var(--color-text-primary);
		background: var(--color-bg-hover);
	}
	.banner-close svg {
		width: 16px;
		height: 16px;
	}
</style>
