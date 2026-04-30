<script lang="ts">
	import { Modal } from '$lib/components/ui';
	import type { UsageLevel } from '$lib/types/usageLevel';
	import { USAGE_LEVEL_LABELS } from '$lib/types/usageLevel';

	interface Props {
		value: UsageLevel;
		saving: boolean;
		onSelect: (level: UsageLevel) => void | Promise<void>;
	}

	let { value, saving, onSelect }: Props = $props();

	type LevelOption = {
		value: UsageLevel;
		title: string;
		summary: string;
		includes: string[];
	};

	const OPTIONS: LevelOption[] = [
		{
			value: 'basic',
			title: USAGE_LEVEL_LABELS.basic,
			summary: 'Только туннели',
			includes: [
				'AmneziaWG туннели',
				'Системные WireGuard туннели',
				'Диагностика',
			],
		},
		{
			value: 'advanced',
			title: USAGE_LEVEL_LABELS.advanced,
			summary: 'Туннели, серверы, маршрутизация',
			includes: [
				'Всё из Базового',
				'Sing-box туннели',
				'Серверы (managed WireGuard)',
				'Маршрутизация: политики, клиенты, DNS, IP, Device Proxy',
				'Мониторинг',
				'Веб-терминал',
			],
		},
		{
			value: 'expert',
			title: USAGE_LEVEL_LABELS.expert,
			summary: 'Все функции',
			includes: ['Всё из Расширенного', 'HR Neo', 'Sing-box Router'],
		},
	];

	let infoFor = $state<UsageLevel | null>(null);
	const infoOpt = $derived(infoFor ? OPTIONS.find((o) => o.value === infoFor) : null);

	let expanded = $state(false);

	function selectLevel(level: UsageLevel) {
		if (level === value || saving) return;
		void onSelect(level);
	}

	function openInfo(e: Event, level: UsageLevel) {
		e.stopPropagation();
		infoFor = level;
	}
</script>

<div class="card">
	<button
		type="button"
		class="collapsible-header"
		aria-expanded={expanded}
		aria-controls="usage-level-body"
		onclick={() => (expanded = !expanded)}
	>
		<span class="section-label">Уровень использования</span>
		<span class="header-meta">
			<span class="current-level">{USAGE_LEVEL_LABELS[value]}</span>
			<svg
				class="chevron"
				class:open={expanded}
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="2"
				aria-hidden="true"
			>
				<polyline points="6 9 12 15 18 9" />
			</svg>
		</span>
	</button>

	{#if expanded}
		<div id="usage-level-body" class="collapsible-body">
			<p class="card-hint">
				Скрывает разделы, которые вам не нужны. Данные при понижении уровня не удаляются.
			</p>

			<div
				class="level-grid"
				role="radiogroup"
				aria-label="Уровень использования"
				aria-busy={saving}
			>
				{#each OPTIONS as opt (opt.value)}
					{@const selected = value === opt.value}
					<button
						type="button"
						role="radio"
						aria-checked={selected}
						class="level-card"
						class:selected
						disabled={saving}
						onclick={() => selectLevel(opt.value)}
					>
						<span
							class="info-btn"
							role="button"
							tabindex="0"
							aria-label={`Подробнее про уровень «${opt.title}»`}
							onclick={(e) => openInfo(e, opt.value)}
							onkeydown={(e) => {
								if (e.key === 'Enter' || e.key === ' ') {
									openInfo(e, opt.value);
								}
							}}
						>
							<svg viewBox="0 0 24 24" aria-hidden="true">
								<circle cx="12" cy="12" r="10" />
								<line x1="12" y1="11" x2="12" y2="17" />
								<circle cx="12" cy="7.5" r="0.8" />
							</svg>
						</span>

						<div class="level-title">{opt.title}</div>

						{#if selected}
							<span class="level-check" aria-hidden="true">
								<svg viewBox="0 0 24 24"><polyline points="20 6 9 17 4 12" /></svg>
							</span>
						{/if}
					</button>
				{/each}
			</div>
		</div>
	{/if}
</div>

<Modal
	open={infoFor !== null}
	title={infoOpt ? `Уровень: ${infoOpt.title}` : ''}
	size="md"
	onclose={() => (infoFor = null)}
>
	{#if infoOpt}
		<p>{infoOpt.summary}</p>
		<h3>Что включает</h3>
		<ul>
			{#each infoOpt.includes as item}
				<li>{item}</li>
			{/each}
		</ul>
	{/if}
</Modal>

<style>
	.collapsible-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		width: 100%;
		background: transparent;
		border: 0;
		padding: 0;
		margin: 0;
		color: inherit;
		font: inherit;
		cursor: pointer;
		text-align: left;
	}
	.collapsible-header:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 2px;
		border-radius: var(--radius-sm);
	}
	.header-meta {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		color: var(--color-text-muted);
		font-size: 0.8125rem;
	}
	.current-level {
		color: var(--color-text-secondary);
	}
	.chevron {
		width: 14px;
		height: 14px;
		transition: transform var(--t-fast) ease;
	}
	.chevron.open {
		transform: rotate(180deg);
	}

	.collapsible-body {
		margin-top: 0.75rem;
	}
	.card-hint {
		color: var(--color-text-muted);
		font-size: 0.8125rem;
		margin: 0 0 0.75rem 0;
	}

	.level-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.5rem;
	}
	@media (max-width: 480px) {
		.level-grid {
			grid-template-columns: 1fr;
		}
	}

	.level-card {
		position: relative;
		text-align: center;
		padding: 0.625rem 0.5rem;
		background: var(--color-bg-tertiary);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-md);
		color: inherit;
		font: inherit;
		cursor: pointer;
		transition:
			border-color var(--t-fast) ease,
			background var(--t-fast) ease;
	}
	.level-card:hover:not(:disabled):not(.selected) {
		background: var(--color-bg-hover);
		border-color: var(--color-border-strong);
	}
	.level-card:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 2px;
	}
	.level-card.selected {
		border-color: var(--color-accent);
		background: var(--color-accent-tint);
	}
	.level-card:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.level-title {
		font-weight: 600;
		font-size: 0.875rem;
		padding: 0 1.25rem;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.level-check {
		position: absolute;
		top: 0.375rem;
		left: 0.375rem;
		width: 14px;
		height: 14px;
		color: var(--color-accent);
	}
	.level-check svg {
		width: 100%;
		height: 100%;
		fill: none;
		stroke: currentColor;
		stroke-width: 2;
	}

	.info-btn {
		position: absolute;
		top: 0.375rem;
		right: 0.375rem;
		width: 14px;
		height: 14px;
		color: var(--color-text-muted);
		cursor: pointer;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		border-radius: 50%;
	}
	.info-btn:hover {
		color: var(--color-text-primary);
	}
	.info-btn:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 2px;
	}
	.info-btn svg {
		width: 12px;
		height: 12px;
		fill: none;
		stroke: currentColor;
		stroke-width: 2;
	}
</style>
