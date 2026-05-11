<script lang="ts">
	import { singboxWizard } from '$lib/stores/singboxWizard';
	import SingboxLogsPanel from './SingboxLogsPanel.svelte';

	interface Props {
		onRetry: () => void;
	}
	let { onRetry }: Props = $props();

	const wizardState = singboxWizard.state;

	function close(): void { singboxWizard.close(); }
	function openLogs(): void {
		window.open('/logs?bucket=singbox', '_blank');
	}
</script>

<div class="title">Не удалось применить</div>
<div class="hint">
	Часть конфигурации применена.
	{#if $wizardState.error?.phase === 'enableEngine'}
		Sing-box не запустился — проверьте логи ниже и нажмите Повторить.
	{:else}
		Вы можете нажать Повторить или закрыть мастер.
	{/if}
</div>

{#if $wizardState.error}
	<div class="err">
		{$wizardState.error.phase}: {$wizardState.error.message}
	</div>
{/if}

<div class="logs-section">
	<div class="logs-head">Логи sing-box</div>
	<SingboxLogsPanel bufferSize={50} />
</div>

<div class="actions">
	<button type="button" class="btn ghost" onclick={openLogs}>Открыть Журнал</button>
	<button type="button" class="btn ghost" onclick={close}>Закрыть</button>
	<button type="button" class="btn primary" onclick={onRetry}>Повторить</button>
</div>

<style>
	.title { color: var(--color-text-primary); font-weight: 600; margin-bottom: 0.4rem; }
	.hint { color: var(--color-text-muted); font-size: 0.85rem; margin-bottom: 0.75rem; }
	.err {
		background: rgba(248,81,73,0.08);
		border-left: 3px solid #f85149;
		padding: 0.5rem 0.75rem;
		border-radius: 4px;
		font-family: var(--font-mono, ui-monospace, monospace);
		font-size: 0.78rem;
		color: var(--color-text-primary);
		line-height: 1.5;
		margin-bottom: 0.75rem;
	}
	.logs-section { margin-bottom: 0.75rem; }
	.logs-head {
		font-size: 0.7rem;
		text-transform: uppercase;
		color: var(--color-text-muted);
		margin-bottom: 0.25rem;
	}
	.actions { margin-top: 0.5rem; display: flex; gap: 0.5rem; justify-content: center; }
	.btn { padding: 0.4rem 1rem; border-radius: 6px; font: inherit; font-size: 0.85rem; cursor: pointer; border: 1px solid transparent; }
	.ghost { color: var(--color-text-muted); background: transparent; }
	.primary { color: white; background: #238636; border-color: #2ea043; }
</style>
