<script lang="ts">
	import type { Snippet } from 'svelte';
	import { Card, Toggle, Button, StatusDot } from '$lib/components/ui';
	import type { FreeTurnProcessStatus } from '$lib/types';
	import { formatUptime } from './uptime';

	interface Props {
		title: string;
		status?: FreeTurnProcessStatus;
		saving: boolean;
		onToggle: (on: boolean) => void;
		onSave: () => void;
		children: Snippet;
	}

	let { title, status, saving, onToggle, onSave, children }: Props = $props();
</script>

<Card>
	{#snippet header()}
		<div class="ft-card-header">
			<div class="ft-card-title">
				<StatusDot variant={status?.running ? 'success' : 'muted'} pulse={status?.running} />
				<span>{title}</span>
				{#if status?.running}
					<span class="ft-uptime">запущен · {formatUptime(status.startedAt)}</span>
				{/if}
			</div>
			<Toggle
				checked={!!status?.running}
				onchange={onToggle}
				disabled={status ? !status.binaryPresent : false}
				label=""
			/>
		</div>
	{/snippet}

	{#if status && !status.binaryPresent}
		<div class="ft-binary-warn">
			Бинарь <code>{status.binary}</code> не найден — awg-manager не поставляет freeturn.
			Установите его из
			<a href="https://github.com/samosvalishe/free-turn-proxy" target="_blank" rel="noopener">free-turn-proxy</a>
			и обновите страницу.
		</div>
	{/if}

	{#if !status?.running && status?.lastError}
		<div class="ft-section-label" style="margin-top: 0">Ошибка последнего запуска</div>
		<pre class="ft-log-box ft-error-box">{status.lastError}</pre>
	{/if}

	{@render children()}

	{#if status?.log}
		<!-- Лог внизу и свёрнут: развёрнутый — выталкивал форму настроек за экран. -->
		<details class="ft-log-details">
			<summary>Лог процесса</summary>
			<pre class="ft-log-box">{status.log}</pre>
		</details>
	{/if}

	{#snippet footer()}
		<div class="ft-footer">
			<Button variant="primary" size="sm" loading={saving} onclick={onSave}>Сохранить</Button>
		</div>
	{/snippet}
</Card>

<style>
	.ft-card-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
	}

	.ft-card-title {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-weight: 500;
		flex-wrap: wrap;
	}

	.ft-uptime {
		font-size: 0.75rem;
		color: var(--color-text-secondary);
		font-weight: 400;
	}

	.ft-binary-warn {
		padding: 0.625rem 0.75rem;
		border-radius: var(--radius-sm);
		border: 1px solid var(--color-warning);
		background: var(--color-warning-tint);
		color: var(--color-text-primary);
		font-size: 0.8125rem;
		margin-bottom: 0.75rem;
	}

	.ft-binary-warn a {
		color: inherit;
		text-decoration: underline;
	}

	.ft-section-label {
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--color-text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.03em;
		margin: 1.25rem 0 0.625rem;
	}

	.ft-error-box {
		color: var(--color-error);
		border-color: var(--color-error);
	}

	.ft-log-details {
		margin-top: 1rem;
	}

	.ft-log-details summary {
		cursor: pointer;
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--color-text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.03em;
		user-select: none;
	}

	.ft-log-details[open] summary {
		margin-bottom: 0.625rem;
	}

	.ft-log-box {
		width: 100%;
		max-height: 160px;
		overflow-y: auto;
		padding: 0.5rem 0.625rem;
		border-radius: var(--radius-sm);
		border: 1px solid var(--color-border);
		background: var(--color-bg-tertiary);
		color: var(--color-text-secondary);
		font-family: monospace;
		font-size: 0.75rem;
		white-space: pre-wrap;
		word-break: break-all;
		margin: 0;
	}

	.ft-footer {
		display: flex;
		justify-content: flex-end;
	}
</style>
