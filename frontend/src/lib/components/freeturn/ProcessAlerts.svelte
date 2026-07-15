<script lang="ts">
	import { Button } from '$lib/components/ui';
	import type { FreeTurnProcessStatus } from '$lib/types';

	interface Props {
		status?: FreeTurnProcessStatus;
		/** Установка в один клик: пин для этой архитектуры есть в билде */
		installAvailable: boolean;
		installVersion?: string;
		installing: boolean;
		onInstall: () => void;
	}

	let { status, installAvailable, installVersion, installing, onInstall }: Props = $props();
</script>

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
	<pre class="ft-error-box">{status.lastError}</pre>
{/if}

<style>
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
		margin-bottom: 0.875rem;
	}

	.ft-binary-warn a {
		color: inherit;
		text-decoration: underline;
	}

	.ft-error-box {
		width: 100%;
		box-sizing: border-box;
		max-height: 160px;
		overflow-y: auto;
		padding: 0.5rem 0.625rem;
		border-radius: var(--radius-sm);
		border: 1px solid var(--color-error);
		background: var(--color-bg-secondary);
		color: var(--color-error);
		font-family: var(--font-mono);
		font-size: 0.75rem;
		white-space: pre-wrap;
		word-break: break-all;
		margin: 0 0 0.875rem;
	}
</style>
