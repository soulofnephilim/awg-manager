<script lang="ts">
	import { AlertCircle } from 'lucide-svelte';
	import { Button } from '$lib/components/ui';

	interface Props {
		count: number;
		/** «клиента» / «сервера» — чей перезапуск применит изменения. */
		target: string;
		saving: boolean;
		onSave: () => void;
		onRevert: () => void;
	}

	let { count, target, saving, onSave, onRevert }: Props = $props();
</script>

<div class="ft-unsaved">
	<span class="ft-unsaved-icon"><AlertCircle size={14} /></span>
	<span class="ft-unsaved-text">
		Несохранённые изменения: {count} — применятся после перезапуска {target}
	</span>
	<Button variant="ghost" size="sm" onclick={onRevert}>Отменить</Button>
	<Button variant="primary" size="sm" loading={saving} onclick={onSave}>Сохранить</Button>
</div>

<style>
	.ft-unsaved {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.625rem;
		padding: 0.5rem 0.875rem;
		border: 1px solid var(--color-warning-border);
		background: color-mix(in srgb, var(--color-warning) 8%, transparent);
		border-radius: var(--radius-sm);
		margin-bottom: 0.875rem;
	}

	.ft-unsaved-icon {
		display: inline-flex;
		color: var(--color-warning);
		flex: none;
	}

	.ft-unsaved-text {
		flex: 1;
		min-width: 0;
		font-size: 0.75rem;
		color: var(--color-warning);
	}
</style>
