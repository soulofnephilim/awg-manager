<script lang="ts">
	import { Link2 } from 'lucide-svelte';
	import { Toggle, Button, StatusDot } from '$lib/components/ui';
	import type { FreeTurnProcessStatus } from '$lib/types';

	interface Props {
		/** «Клиент» / «Сервер» — статус запуска дописывается в заголовке. */
		title: string;
		status?: FreeTurnProcessStatus;
		/** Части мета-строки при running (uptime, PID, режим, обфускация). */
		metaParts: string[];
		actionLabel: string;
		logOpen: boolean;
		/** Установка в один клик: пин для этой архитектуры есть в билде */
		installAvailable: boolean;
		installVersion?: string;
		installing: boolean;
		onInstall: () => void;
		onAction: () => void;
		onToggle: (on: boolean) => void;
		onToggleLog: () => void;
	}

	let {
		title,
		status,
		metaParts,
		actionLabel,
		logOpen,
		installAvailable,
		installVersion,
		installing,
		onInstall,
		onAction,
		onToggle,
		onToggleLog
	}: Props = $props();
</script>

<div class="ft-hero">
	<StatusDot variant={status?.running ? 'success' : 'muted'} pulse={status?.running} />
	<div class="ft-hero-text">
		<div class="ft-hero-title">{title} {status?.running ? 'запущен' : 'остановлен'}</div>
		<div class="ft-hero-meta">
			{status?.running ? metaParts.join(' · ') : 'процесс не запущен — включите тумблером'}
		</div>
	</div>
	<div class="ft-hero-actions">
		<Button variant="primary" size="sm" onclick={onAction}>
			{#snippet iconBefore()}<Link2 size={14} />{/snippet}
			{actionLabel}
		</Button>
		<Button variant="ghost" size="sm" onclick={onToggleLog}>Лог</Button>
		<Toggle
			checked={!!status?.running}
			onchange={onToggle}
			disabled={status ? !status.binaryPresent : false}
			controlled
			label=""
			ariaLabel="{title}: запустить или остановить"
		/>
	</div>
</div>

{#if status && !status.binaryPresent}
	<div class="ft-binary-warn">
		<span>
			Бинарь <code>{status.binary}</code> не найден — awg-manager не поставляет freeturn
			в своём пакете.
			{#if !installAvailable}
				Установите его вручную из
				<a href="https://github.com/samosvalishe/free-turn-proxy" target="_blank" rel="noopener">free-turn-proxy</a>
				и обновите страницу.
			{/if}
		</span>
		{#if installAvailable}
			<Button variant="secondary" size="sm" loading={installing} onclick={onInstall}>
				Установить v{installVersion} (клиент + сервер)
			</Button>
		{/if}
	</div>
{/if}

{#if !status?.running && status?.lastError}
	<div class="section-label">Ошибка последнего запуска</div>
	<pre class="ft-log-box ft-error-box">{status.lastError}</pre>
{/if}

{#if logOpen}
	<pre class="ft-log-box">{status?.log || 'лог пуст'}</pre>
{/if}

<style>
	.ft-hero {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 1.125rem 1.25rem;
		background: var(--color-bg-secondary);
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		margin-bottom: 0.625rem;
	}

	.ft-hero-text {
		flex: 1;
		min-width: 0;
	}

	.ft-hero-title {
		font-size: 1rem;
		font-weight: 600;
	}

	.ft-hero-meta {
		font-size: 0.75rem;
		color: var(--color-text-secondary);
		font-family: var(--font-mono);
	}

	.ft-hero-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex: none;
	}

	.ft-binary-warn {
		display: flex;
		align-items: center;
		justify-content: space-between;
		flex-wrap: wrap;
		gap: 0.625rem;
		padding: 0.625rem 0.75rem;
		border-radius: var(--radius-sm);
		border: 1px solid var(--color-warning);
		background: var(--color-warning-tint);
		color: var(--color-text-primary);
		font-size: 0.8125rem;
		margin-bottom: 0.625rem;
	}

	.ft-binary-warn a {
		color: inherit;
		text-decoration: underline;
	}

	.ft-log-box {
		width: 100%;
		box-sizing: border-box;
		max-height: 160px;
		overflow-y: auto;
		padding: 0.5rem 0.625rem;
		border-radius: var(--radius-sm);
		border: 1px solid var(--color-border);
		background: var(--color-bg-secondary);
		color: var(--color-text-secondary);
		font-family: var(--font-mono);
		font-size: 0.75rem;
		white-space: pre-wrap;
		word-break: break-all;
		margin: 0 0 0.625rem;
	}

	.ft-error-box {
		color: var(--color-error);
		border-color: var(--color-error);
	}

	@media (max-width: 640px) {
		.ft-hero {
			flex-wrap: wrap;
		}

		.ft-hero-text {
			flex-basis: calc(100% - 12px - 1rem);
		}
	}
</style>
